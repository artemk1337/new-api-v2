package model

import (
	"errors"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PricingSyncStrategyHighest = "highest"
	PricingSyncStrategyLowest  = "lowest"
	PricingSyncStrategyAverage = "average"

	PricingSyncModelModeManual  = "manual"
	PricingSyncModelModeGeneral = "general"
	PricingSyncModelModeChannel = "channel"

	PricingSyncModelStatusReady       = "ready"
	PricingSyncModelStatusConflict    = "conflict"
	PricingSyncModelStatusStale       = "stale"
	PricingSyncModelStatusUnavailable = "unavailable"
)

var (
	ErrPricingSyncSourceNotSelected    = errors.New("pricing sync source is not selected")
	ErrPricingSyncConfigurationChanged = errors.New("pricing sync configuration changed; retry configuration update")
)

var pricingSyncIntervals = map[int]struct{}{
	0: {}, 60: {}, 10 * 60: {}, 30 * 60: {}, 60 * 60: {},
}

// PricingSyncSource is a persisted upstream source. ChannelID is also used by
// the two built-in negative-ID presets, so it intentionally has no FK.
type PricingSyncSource struct {
	ChannelID       int    `json:"channel_id" gorm:"primaryKey"`
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint" gorm:"type:text"`
	IntervalSeconds int    `json:"interval_seconds"`
	LastAttemptAt   int64  `json:"last_attempt_at"`
	LastSuccessAt   int64  `json:"last_success_at"`
	LastError       string `json:"last_error,omitempty" gorm:"type:text"`
}

// PricingSyncQuote stores the last confirmed complete billing contract from
// one source. Data is canonical JSON produced by the ratio-sync parser.
type PricingSyncQuote struct {
	ChannelID     int    `json:"channel_id" gorm:"primaryKey"`
	ModelName     string `json:"model_name" gorm:"primaryKey;size:255"`
	Category      string `json:"category" gorm:"size:32"`
	Data          string `json:"data" gorm:"type:text"`
	CandidateHash string `json:"candidate_hash" gorm:"size:128"`
	Confirmations int    `json:"confirmations"`
	MissingCount  int    `json:"missing_count"`
	ConfirmedAt   int64  `json:"confirmed_at"`
}

// PricingSyncModelState records how a model is resolved and which source(s)
// own its automatically applied price. Empty rows mean the global rule.
type PricingSyncModelState struct {
	ModelName       string `json:"model_name" gorm:"primaryKey;size:255"`
	Mode            string `json:"mode" gorm:"size:32"`
	ChannelID       int    `json:"channel_id"`
	Provenance      string `json:"provenance,omitempty" gorm:"type:text"`
	Status          string `json:"status" gorm:"size:32"`
	ConflictDetails string `json:"conflict_details,omitempty" gorm:"type:text"`
	LastAppliedAt   int64  `json:"last_applied_at"`
}

type PricingSyncModelPreferenceInput struct {
	ModelName string
	Mode      string
	ChannelID int
}

func ValidatePricingSyncSource(source PricingSyncSource) error {
	if source.ChannelID == 0 {
		return errors.New("pricing sync source channel_id is required")
	}
	if _, ok := pricingSyncIntervals[source.IntervalSeconds]; !ok {
		return errors.New("unsupported pricing sync interval")
	}
	endpoint := strings.TrimSpace(source.Endpoint)
	if endpoint == "" {
		return errors.New("pricing sync endpoint is required")
	}
	if endpoint == "openrouter" {
		return nil
	}
	if (source.ChannelID == -100 && endpoint == "https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json") ||
		(source.ChannelID == -101 && endpoint == "https://models.dev/api.json") {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(endpoint, "//") {
		return errors.New("pricing sync custom endpoint must be relative")
	}
	return nil
}

func GetPricingSyncSources() ([]PricingSyncSource, error) {
	sources := make([]PricingSyncSource, 0)
	if err := DB.Order("channel_id ASC").Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

func SavePricingSyncSources(sources []PricingSyncSource) error {
	unique := make(map[int]PricingSyncSource, len(sources))
	for _, source := range sources {
		source.Endpoint = strings.TrimSpace(source.Endpoint)
		if err := ValidatePricingSyncSource(source); err != nil {
			return err
		}
		if _, exists := unique[source.ChannelID]; exists {
			return errors.New("duplicate pricing sync source")
		}
		unique[source.ChannelID] = source
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PricingSyncSource{}).Error; err != nil {
			return err
		}
		if len(sources) == 0 {
			return nil
		}
		return tx.Create(&sources).Error
	})
}

func SavePricingSyncConfiguration(sources []PricingSyncSource, strategy string, removedSourceIDs []int) error {
	return savePricingSyncConfiguration(sources, strategy, removedSourceIDs, nil, -1)
}

func SavePricingSyncConfigurationIfVersion(sources []PricingSyncSource, strategy string, removedSourceIDs []int, expectedPrevious []PricingSyncSource, expectedVersion int64) error {
	return savePricingSyncConfiguration(sources, strategy, removedSourceIDs, expectedPrevious, expectedVersion)
}

func savePricingSyncConfiguration(sources []PricingSyncSource, strategy string, removedSourceIDs []int, expectedPrevious []PricingSyncSource, expectedVersion int64) error {
	switch strategy {
	case PricingSyncStrategyHighest, PricingSyncStrategyLowest, PricingSyncStrategyAverage:
	default:
		return errors.New("unsupported pricing sync strategy")
	}
	existing, err := GetPricingSyncSources()
	if err != nil {
		return err
	}
	existingByID := lo.SliceToMap(existing, func(source PricingSyncSource) (int, PricingSyncSource) {
		return source.ChannelID, source
	})
	seen := make(map[int]struct{}, len(sources))
	for index := range sources {
		sources[index].Endpoint = strings.TrimSpace(sources[index].Endpoint)
		if err := ValidatePricingSyncSource(sources[index]); err != nil {
			return err
		}
		if _, ok := seen[sources[index].ChannelID]; ok {
			return errors.New("duplicate pricing sync source")
		}
		seen[sources[index].ChannelID] = struct{}{}
		previous, ok := existingByID[sources[index].ChannelID]
		if ok && previous.Enabled == sources[index].Enabled &&
			previous.Endpoint == sources[index].Endpoint &&
			previous.IntervalSeconds == sources[index].IntervalSeconds {
			sources[index].LastAttemptAt = previous.LastAttemptAt
			sources[index].LastSuccessAt = previous.LastSuccessAt
			sources[index].LastError = previous.LastError
		}
	}

	removed := lo.SliceToMap(removedSourceIDs, func(channelID int) (int, struct{}) {
		return channelID, struct{}{}
	})
	ownsRemovedSource := func(state PricingSyncModelState) bool {
		if state.Mode == PricingSyncModelModeChannel {
			if _, ok := removed[state.ChannelID]; ok {
				return true
			}
		}
		var provenance []int
		if common.UnmarshalJsonStr(state.Provenance, &provenance) != nil {
			return false
		}
		return lo.SomeBy(provenance, func(channelID int) bool {
			_, ok := removed[channelID]
			return ok
		})
	}
	allStates := make([]PricingSyncModelState, 0)
	if err := DB.Where("mode <> ?", PricingSyncModelModeManual).Find(&allStates).Error; err != nil {
		return err
	}
	ownedStates := lo.Filter(allStates, func(state PricingSyncModelState, _ int) bool {
		return ownsRemovedSource(state)
	})
	patches := make(map[string]JSONObjectPatch)
	if len(ownedStates) > 0 {
		names := lo.Map(ownedStates, func(state PricingSyncModelState, _ int) string { return state.ModelName })
		for key := range jsonObjectPatchOptionKeys {
			patches[key] = JSONObjectPatch{Delete: names}
		}
	}
	err = ApplyJSONOptionPatchesWithTx(patches, func(tx *gorm.DB) error {
		if expectedVersion >= 0 {
			version := Option{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&version, "key = ?", "PricingSyncConfigVersion").Error; err != nil {
				return err
			}
			if version.Value != strconv.FormatInt(expectedVersion, 10) {
				return ErrPricingSyncConfigurationChanged
			}
		}
		if err := bumpPricingSyncConfigVersionTx(tx); err != nil {
			return err
		}
		currentSources := make([]PricingSyncSource, 0)
		if err := tx.Find(&currentSources).Error; err != nil {
			return err
		}
		if expectedPrevious != nil {
			previous := append([]PricingSyncSource(nil), expectedPrevious...)
			sort.Slice(currentSources, func(i, j int) bool { return currentSources[i].ChannelID < currentSources[j].ChannelID })
			sort.Slice(previous, func(i, j int) bool { return previous[i].ChannelID < previous[j].ChannelID })
			if len(currentSources) != len(previous) {
				return ErrPricingSyncConfigurationChanged
			}
			for index := range currentSources {
				current, expected := currentSources[index], previous[index]
				if current.ChannelID != expected.ChannelID || current.Enabled != expected.Enabled || current.Endpoint != expected.Endpoint || current.IntervalSeconds != expected.IntervalSeconds {
					return ErrPricingSyncConfigurationChanged
				}
			}
		}
		currentByID := lo.SliceToMap(currentSources, func(source PricingSyncSource) (int, PricingSyncSource) { return source.ChannelID, source })
		channelIDs := lo.FilterMap(sources, func(source PricingSyncSource, _ int) (int, bool) {
			return source.ChannelID, source.ChannelID > 0
		})
		if len(channelIDs) > 0 {
			var channels []Channel
			if err := tx.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
				return err
			}
			if len(channels) != len(channelIDs) {
				return ErrPricingSyncConfigurationChanged
			}
		}
		for index := range sources {
			if current, ok := currentByID[sources[index].ChannelID]; ok && current.Enabled == sources[index].Enabled && current.Endpoint == sources[index].Endpoint && current.IntervalSeconds == sources[index].IntervalSeconds {
				sources[index].LastAttemptAt = current.LastAttemptAt
				sources[index].LastSuccessAt = current.LastSuccessAt
				sources[index].LastError = current.LastError
			}
		}
		currentStates := make([]PricingSyncModelState, 0)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("mode <> ?", PricingSyncModelModeManual).Find(&currentStates).Error; err != nil {
			return err
		}
		currentOwnedNames := lo.Map(
			lo.Filter(currentStates, func(state PricingSyncModelState, _ int) bool {
				return ownsRemovedSource(state)
			}),
			func(state PricingSyncModelState, _ int) string { return state.ModelName },
		)
		expectedOwnedNames := lo.Map(ownedStates, func(state PricingSyncModelState, _ int) string {
			return state.ModelName
		})
		sort.Strings(currentOwnedNames)
		sort.Strings(expectedOwnedNames)
		if !slices.Equal(currentOwnedNames, expectedOwnedNames) {
			return ErrPricingSyncConfigurationChanged
		}
		strategyOption := Option{Key: "PricingSyncStrategy", Value: strategy}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&strategyOption).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PricingSyncSource{}).Error; err != nil {
			return err
		}
		if len(sources) > 0 {
			if err := tx.Create(&sources).Error; err != nil {
				return err
			}
		}
		if len(removedSourceIDs) > 0 {
			if err := tx.Where("channel_id IN ?", removedSourceIDs).Delete(&PricingSyncQuote{}).Error; err != nil {
				return err
			}
		}
		if len(ownedStates) > 0 {
			names := lo.Map(ownedStates, func(state PricingSyncModelState, _ int) string { return state.ModelName })
			if err := tx.Model(&PricingSyncModelState{}).Where("model_name IN ?", names).Updates(map[string]any{
				"mode": PricingSyncModelModeManual, "channel_id": 0, "provenance": "",
				"conflict_details": "", "status": PricingSyncModelStatusUnavailable,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return updateOptionMapFromDatabase("PricingSyncStrategy", strategy)
}

func bumpPricingSyncConfigVersionTx(tx *gorm.DB) error {
	version := Option{Key: "PricingSyncConfigVersion", Value: "0"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&version).Error; err != nil {
		return err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&version, "key = ?", version.Key).Error; err != nil {
		return err
	}
	currentVersion, err := strconv.ParseInt(version.Value, 10, 64)
	if err != nil {
		return err
	}
	version.Value = strconv.FormatInt(currentVersion+1, 10)
	return tx.Save(&version).Error
}

func GetPricingSyncConfigVersion() (int64, error) {
	option := Option{Key: "PricingSyncConfigVersion", Value: "0"}
	if err := DB.FirstOrCreate(&option, Option{Key: option.Key}).Error; err != nil {
		return 0, err
	}
	version, err := strconv.ParseInt(option.Value, 10, 64)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func GetPricingSyncStrategy() string {
	common.OptionMapRWMutex.RLock()
	strategy := strings.TrimSpace(common.OptionMap["PricingSyncStrategy"])
	common.OptionMapRWMutex.RUnlock()
	switch strategy {
	case PricingSyncStrategyLowest, PricingSyncStrategyAverage, PricingSyncStrategyHighest:
		return strategy
	default:
		return PricingSyncStrategyHighest
	}
}

func GetPricingSyncMinimumInterval() int {
	sources, err := GetPricingSyncSources()
	if err != nil {
		return 0
	}
	minimum := 0
	for _, source := range sources {
		if !source.Enabled || source.IntervalSeconds == 0 {
			continue
		}
		if minimum == 0 || source.IntervalSeconds < minimum {
			minimum = source.IntervalSeconds
		}
	}
	return minimum
}

func PricingSyncSourceIDs(sources []PricingSyncSource) []int {
	ids := make([]int, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ChannelID)
	}
	sort.Ints(ids)
	return ids
}

func GetPricingSyncModelState(modelName string) (*PricingSyncModelState, error) {
	state := &PricingSyncModelState{}
	err := DB.First(state, "model_name = ?", strings.TrimSpace(modelName)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &PricingSyncModelState{
			ModelName: strings.TrimSpace(modelName),
			Mode:      PricingSyncModelModeGeneral,
			Status:    PricingSyncModelStatusReady,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return state, nil
}

func GetPricingSyncModelStates() (map[string]PricingSyncModelState, error) {
	states := make([]PricingSyncModelState, 0)
	if err := DB.Find(&states).Error; err != nil {
		return nil, err
	}
	result := make(map[string]PricingSyncModelState, len(states))
	for _, state := range states {
		result[state.ModelName] = state
	}
	return result, nil
}

func SavePricingSyncModelState(state PricingSyncModelState) error {
	state.ModelName = strings.TrimSpace(state.ModelName)
	if state.ModelName == "" {
		return errors.New("pricing sync model name is required")
	}
	switch state.Mode {
	case PricingSyncModelModeManual, PricingSyncModelModeGeneral:
		state.ChannelID = 0
	case PricingSyncModelModeChannel:
		if state.ChannelID == 0 {
			return errors.New("pricing sync model channel is required")
		}
	default:
		return errors.New("unsupported pricing sync model mode")
	}
	if state.Status == "" {
		state.Status = PricingSyncModelStatusReady
	}
	return DB.Save(&state).Error
}

// DisablePricingSyncSources clears prices owned by an explicitly selected
// source. It deliberately does not silently fall back to another channel:
// using a potentially cheaper replacement can undercharge real upstream cost.
func DisablePricingSyncSources(channelIDs []int) error {
	return disablePricingSyncSourcesWithMutation(channelIDs, nil)
}

// disablePricingSyncSourcesWithMutation keeps channel deletion and all
// pricing-source cleanup in one transaction. The optional mutation must use
// the supplied transaction and must not update in-memory state.
func disablePricingSyncSourcesWithMutation(channelIDs []int, mutation func(*gorm.DB) error) error {
	return disablePricingSyncSourcesWithMutationChecked(channelIDs, nil, mutation)
}

func disablePricingSyncSourcesWithMutationChecked(channelIDs []int, check func(*gorm.DB) error, mutation func(*gorm.DB) error) error {
	if len(channelIDs) == 0 {
		if mutation != nil {
			return DB.Transaction(mutation)
		}
		return nil
	}
	removed := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		removed[channelID] = struct{}{}
	}
	ownsRemovedSource := func(state PricingSyncModelState) bool {
		if state.Mode == PricingSyncModelModeChannel {
			if _, ok := removed[state.ChannelID]; ok {
				return true
			}
		}
		var provenance []int
		if common.UnmarshalJsonStr(state.Provenance, &provenance) != nil {
			return false
		}
		return lo.ContainsBy(provenance, func(channelID int) bool {
			_, ok := removed[channelID]
			return ok
		})
	}
	allStates := make([]PricingSyncModelState, 0)
	if err := DB.Where("mode <> ?", PricingSyncModelModeManual).Find(&allStates).Error; err != nil {
		return err
	}
	states := lo.Filter(allStates, func(state PricingSyncModelState, _ int) bool {
		return ownsRemovedSource(state)
	})
	modelNames := lo.Map(states, func(state PricingSyncModelState, _ int) string {
		return state.ModelName
	})
	patches := make(map[string]JSONObjectPatch)
	if len(modelNames) > 0 {
		for key := range jsonObjectPatchOptionKeys {
			patches[key] = JSONObjectPatch{Delete: modelNames}
		}
	}
	return ApplyJSONOptionPatchesWithTx(patches, func(tx *gorm.DB) error {
		if err := bumpPricingSyncConfigVersionTx(tx); err != nil {
			return err
		}
		currentStates := make([]PricingSyncModelState, 0)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("mode <> ?", PricingSyncModelModeManual).Find(&currentStates).Error; err != nil {
			return err
		}
		currentNames := lo.Map(
			lo.Filter(currentStates, func(state PricingSyncModelState, _ int) bool {
				return ownsRemovedSource(state)
			}),
			func(state PricingSyncModelState, _ int) string { return state.ModelName },
		)
		sort.Strings(currentNames)
		sort.Strings(modelNames)
		if !slices.Equal(currentNames, modelNames) {
			return errors.New("pricing sync state changed; retry source deletion")
		}
		if check != nil {
			if err := check(tx); err != nil {
				return err
			}
		}
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&PricingSyncSource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&PricingSyncQuote{}).Error; err != nil {
			return err
		}
		if len(modelNames) > 0 {
			if err := tx.Model(&PricingSyncModelState{}).Where("model_name IN ?", modelNames).Updates(map[string]any{
				"mode": PricingSyncModelModeManual, "channel_id": 0, "provenance": "",
				"conflict_details": "", "status": PricingSyncModelStatusUnavailable,
			}).Error; err != nil {
				return err
			}
		}
		if mutation != nil {
			return mutation(tx)
		}
		return nil
	})
}

func MarkPricingSyncSourcesStale(channelIDs []int) error {
	if len(channelIDs) == 0 {
		return nil
	}
	states := make([]PricingSyncModelState, 0)
	if err := DB.Where("mode <> ?", PricingSyncModelModeManual).Find(&states).Error; err != nil {
		return err
	}
	names := make([]string, 0)
	for _, state := range states {
		if state.Mode == PricingSyncModelModeChannel && lo.Contains(channelIDs, state.ChannelID) {
			names = append(names, state.ModelName)
			continue
		}
		var provenance []int
		if common.UnmarshalJsonStr(state.Provenance, &provenance) == nil &&
			lo.SomeBy(provenance, func(channelID int) bool { return lo.Contains(channelIDs, channelID) }) {
			names = append(names, state.ModelName)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return DB.Model(&PricingSyncModelState{}).Where("model_name IN ?", names).
		Update("status", PricingSyncModelStatusStale).Error
}

func MarkPricingSyncModelsManual(modelNames []string) error {
	modelNames = lo.Uniq(lo.FilterMap(modelNames, func(name string, _ int) (string, bool) {
		name = strings.TrimSpace(name)
		return name, name != ""
	}))
	if len(modelNames) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, modelName := range modelNames {
			state := PricingSyncModelState{
				ModelName: modelName,
				Mode:      PricingSyncModelModeManual,
				Status:    PricingSyncModelStatusReady,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "model_name"}},
				DoUpdates: clause.Assignments(map[string]any{
					"mode": PricingSyncModelModeManual, "channel_id": 0,
					"provenance": "", "conflict_details": "",
					"status": PricingSyncModelStatusReady,
				}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ApplyPricingSyncUpdate(patches map[string]JSONObjectPatch, states []PricingSyncModelState) error {
	return ApplyPricingSyncUpdateIfVersion(patches, states, -1)
}

func ApplyPricingSyncUpdateWithPreferences(patches map[string]JSONObjectPatch, preferences []PricingSyncModelPreferenceInput) error {
	return ApplyJSONOptionPatchesWithTx(patches, func(tx *gorm.DB) error {
		if err := bumpPricingSyncConfigVersionTx(tx); err != nil {
			return err
		}
		if len(preferences) == 0 {
			return nil
		}
		seen := make(map[string]struct{}, len(preferences))
		names := make([]string, 0, len(preferences))
		for _, preference := range preferences {
			name := strings.TrimSpace(preference.ModelName)
			if name == "" {
				return errors.New("pricing preference model is required")
			}
			if _, ok := seen[name]; ok {
				return errors.New("duplicate pricing model preference")
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		states := make(map[string]PricingSyncModelState, len(names))
		var current []PricingSyncModelState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_name IN ?", names).Find(&current).Error; err != nil {
			return err
		}
		for _, state := range current {
			states[state.ModelName] = state
		}
		enabled := make(map[int]struct{})
		for _, preference := range preferences {
			if preference.Mode == PricingSyncModelModeChannel {
				var source PricingSyncSource
				if err := tx.Where("channel_id = ? AND enabled = ?", preference.ChannelID, true).First(&source).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return ErrPricingSyncSourceNotSelected
					}
					return err
				}
				enabled[preference.ChannelID] = struct{}{}
			}
		}
		for _, preference := range preferences {
			name := strings.TrimSpace(preference.ModelName)
			state := PricingSyncModelState{ModelName: name, Mode: preference.Mode, ChannelID: preference.ChannelID, Status: PricingSyncModelStatusReady}
			switch state.Mode {
			case PricingSyncModelModeManual, PricingSyncModelModeGeneral:
				state.ChannelID = 0
			case PricingSyncModelModeChannel:
				if _, ok := enabled[state.ChannelID]; !ok {
					return ErrPricingSyncSourceNotSelected
				}
			default:
				return errors.New("unsupported pricing sync model mode")
			}
			if state.Mode != PricingSyncModelModeManual {
				if previous, ok := states[name]; ok {
					state.Provenance = previous.Provenance
					state.ConflictDetails = previous.ConflictDetails
					state.LastAppliedAt = previous.LastAppliedAt
				}
				state.Status = PricingSyncModelStatusStale
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "model_name"}},
				DoUpdates: clause.AssignmentColumns([]string{"mode", "channel_id", "provenance", "status", "conflict_details", "last_applied_at"}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ApplyPricingSyncUpdateIfVersion(patches map[string]JSONObjectPatch, states []PricingSyncModelState, expectedVersion int64) error {
	return ApplyJSONOptionPatchesWithTx(patches, func(tx *gorm.DB) error {
		if expectedVersion >= 0 {
			version := Option{}
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&version, "key = ?", "PricingSyncConfigVersion").Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if expectedVersion != 0 {
					return errors.New("pricing sync configuration changed")
				}
			} else if err != nil {
				return err
			} else if version.Value != strconv.FormatInt(expectedVersion, 10) {
				return errors.New("pricing sync configuration changed")
			}
		}
		for _, state := range states {
			if state.ModelName == "" {
				continue
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "model_name"}},
				DoUpdates: clause.AssignmentColumns([]string{"mode", "channel_id", "provenance", "status", "conflict_details", "last_applied_at"}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func PricingOptionChangedModels(key, next string) ([]string, error) {
	if _, ok := jsonObjectPatchOptionKeys[key]; !ok {
		return nil, nil
	}
	current := Option{Key: key, Value: "{}"}
	err := DB.First(&current, "key = ?", key).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	before := make(map[string]any)
	after := make(map[string]any)
	if err := common.UnmarshalJsonStr(current.Value, &before); err != nil {
		return nil, err
	}
	if err := common.UnmarshalJsonStr(next, &after); err != nil {
		return nil, err
	}
	changed := make([]string, 0)
	for name, value := range before {
		if nextValue, ok := after[name]; !ok || common.Interface2String(nextValue) != common.Interface2String(value) {
			changed = append(changed, name)
		}
	}
	for name, value := range after {
		if previous, ok := before[name]; !ok || common.Interface2String(previous) != common.Interface2String(value) {
			changed = append(changed, name)
		}
	}
	return lo.Uniq(changed), nil
}
