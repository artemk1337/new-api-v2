package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrSupplierPricingConflict   = errors.New("supplier pricing configuration or revision changed")
	ErrSupplierTariffUnavailable = errors.New("pricing is unavailable for the selected group and model")
)

// SupplierPricingState atomically selects one complete immutable revision.
type SupplierPricingState struct {
	ProfileID               int `gorm:"primaryKey;autoIncrement:false"`
	ActiveRevision          int64
	CandidateHash           string `gorm:"size:64"`
	CandidateConfigRevision int64
	Confirmations           int
}

type SupplierPricingRevision struct {
	ProfileID      int   `gorm:"primaryKey;autoIncrement:false"`
	Revision       int64 `gorm:"primaryKey;autoIncrement:false"`
	ConfigRevision int64
	ContractHash   string `gorm:"size:64"`
	ConfigSnapshot string `gorm:"type:text"`
	CreatedAt      int64
}

type SupplierModelPrice struct {
	ProfileID int    `gorm:"primaryKey;autoIncrement:false"`
	Revision  int64  `gorm:"primaryKey;autoIncrement:false"`
	ModelName string `gorm:"primaryKey;size:255"`
	Data      string `gorm:"type:text"`
}

// ValidateSupplierModelTariff never fills missing lanes from global pricing.
func ValidateSupplierModelTariff(tariff types.SupplierModelTariff) error {
	if strings.TrimSpace(tariff.ModelName) == "" || len(tariff.ModelName) > 255 {
		return errors.New("model_name must contain 1 to 255 bytes")
	}
	for _, price := range []*float64{tariff.InputUSD, tariff.OutputUSD, tariff.CacheReadUSD, tariff.CacheWriteUSD, tariff.CacheWrite1hUSD, tariff.ImageUSD, tariff.ImageOutputUSD, tariff.AudioInputUSD, tariff.AudioOutputUSD, tariff.ReasoningUSD, tariff.PriceUSD} {
		if price != nil && (*price < 0 || math.IsNaN(*price) || math.IsInf(*price, 0)) {
			return errors.New("all tariff prices must be finite and nonnegative")
		}
	}
	for key, price := range tariff.Tools {
		if key == "" || price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return errors.New("tool prices must have a name and finite nonnegative USD cost")
		}
		if key != "web_search" && key != "web_search_preview" && key != "file_search" && key != "image_generation" {
			parts := strings.Split(key, ":")
			if len(parts) != 3 || parts[0] != "image_generation" || parts[1] == "" || parts[2] == "" {
				return errors.New("unsupported supplier tool price key")
			}
		}
	}
	switch tariff.BillingMode {
	case types.SupplierBillingModeToken:
		if tariff.InputUSD == nil || tariff.OutputUSD == nil || tariff.Expression != "" || tariff.PriceUSD != nil || tariff.PriceUnit != "million_tokens" {
			return errors.New("token tariff requires input/output USD and million_tokens unit")
		}
	case types.SupplierBillingModeExpression:
		if tariff.Expression == "" || len(tariff.Expression) > 65536 || tariff.PriceUSD != nil || tariff.PriceUnit != "million_tokens" {
			return errors.New("expression tariff requires a complete expression and million_tokens unit")
		}
		if err := billingexpr.ValidateSupplierExpression(tariff.Expression); err != nil {
			return fmt.Errorf("invalid supplier billing expression: %w", err)
		}
		if err := billing_setting.SmokeTestExpr(tariff.Expression); err != nil {
			return fmt.Errorf("invalid supplier billing expression: %w", err)
		}
	case types.SupplierBillingModeFixed:
		if tariff.PriceUSD == nil || tariff.Expression != "" || (tariff.PriceUnit != "request" && tariff.PriceUnit != "per_second") {
			return errors.New("fixed tariff requires price_usd and request or per_second unit")
		}
	default:
		return errors.New("unsupported supplier billing mode")
	}
	return nil
}

func supplierTariffCandidate(tariffs []types.SupplierModelTariff) ([]types.SupplierModelTariff, string, error) {
	if len(tariffs) > 10000 {
		return nil, "", errors.New("supplier catalogue exceeds 10000 models")
	}
	result := make([]types.SupplierModelTariff, len(tariffs))
	seen := make(map[string]bool, len(tariffs))
	for i, tariff := range tariffs {
		tariff = tariff.Clone()
		tariff.SupplierID, tariff.ProfileID, tariff.Revision = 0, 0, 0
		tariff.ModelName = strings.TrimSpace(tariff.ModelName)
		if err := ValidateSupplierModelTariff(tariff); err != nil {
			return nil, "", fmt.Errorf("%s: %w", tariff.ModelName, err)
		}
		if seen[tariff.ModelName] {
			return nil, "", errors.New("duplicate supplier model contract")
		}
		seen[tariff.ModelName] = true
		result[i] = tariff
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ModelName < result[j].ModelName })
	data, err := common.Marshal(result)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(data)
	return result, hex.EncodeToString(hash[:]), nil
}

func supplierPricingStateTx(tx *gorm.DB, profileID int) (SupplierPricingState, error) {
	state := SupplierPricingState{ProfileID: profileID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return state, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "profile_id = ?", profileID).Error
	return state, err
}

func supplierPricingProfileTx(tx *gorm.DB, profileID int, configRevision int64) (SupplierPricingProfile, error) {
	profile := SupplierPricingProfile{}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, profileID).Error; err != nil {
		return profile, err
	}
	if profile.ConfigRevision != configRevision {
		return profile, ErrSupplierPricingConflict
	}
	return profile, nil
}

func publishSupplierTariffsTx(tx *gorm.DB, state *SupplierPricingState, profile SupplierPricingProfile, tariffs []types.SupplierModelTariff, hash string) (int64, error) {
	if state.ActiveRevision > 0 {
		current := SupplierPricingRevision{}
		if err := tx.First(&current, "profile_id = ? AND revision = ?", state.ProfileID, state.ActiveRevision).Error; err != nil {
			return 0, err
		}
		if current.ContractHash == hash && current.ConfigRevision == profile.ConfigRevision {
			return state.ActiveRevision, nil
		}
	}
	revision := state.ActiveRevision + 1
	basis, err := common.Marshal(struct {
		BaseURL       string  `json:"base_url"`
		Endpoint      string  `json:"endpoint"`
		UpstreamGroup string  `json:"upstream_group"`
		USDPerCredit  float64 `json:"usd_per_credit"`
	}{profile.BaseURL, profile.Endpoint, profile.UpstreamGroup, profile.USDPerCredit})
	if err != nil {
		return 0, err
	}
	row := SupplierPricingRevision{ProfileID: state.ProfileID, Revision: revision, ConfigRevision: profile.ConfigRevision, ContractHash: hash, ConfigSnapshot: string(basis), CreatedAt: time.Now().Unix()}
	if err := tx.Create(&row).Error; err != nil {
		return 0, err
	}
	prices := make([]SupplierModelPrice, 0, len(tariffs))
	for _, tariff := range tariffs {
		data, err := common.Marshal(tariff)
		if err != nil {
			return 0, err
		}
		prices = append(prices, SupplierModelPrice{ProfileID: state.ProfileID, Revision: revision, ModelName: tariff.ModelName, Data: string(data)})
	}
	if len(prices) > 0 {
		if err := tx.CreateInBatches(prices, 100).Error; err != nil {
			return 0, err
		}
	}
	update := tx.Model(&SupplierPricingState{}).Where("profile_id = ? AND active_revision = ?", state.ProfileID, state.ActiveRevision).Updates(map[string]any{"active_revision": revision, "candidate_hash": "", "confirmations": 0})
	if update.Error != nil {
		return 0, update.Error
	}
	if update.RowsAffected != 1 {
		return 0, ErrSupplierPricingConflict
	}
	state.ActiveRevision = revision
	return revision, nil
}

// PublishSupplierProfileTariffs publishes an explicitly reviewed full catalogue.
func PublishSupplierProfileTariffs(profileID int, expectedConfigRevision, expectedRevision int64, tariffs []types.SupplierModelTariff) (int64, error) {
	contracts, hash, err := supplierTariffCandidate(tariffs)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		profile, err := supplierPricingProfileTx(tx, profileID, expectedConfigRevision)
		if err != nil {
			return err
		}
		state, err := supplierPricingStateTx(tx, profileID)
		if err != nil {
			return err
		}
		if state.ActiveRevision != expectedRevision {
			return ErrSupplierPricingConflict
		}
		revision, err = publishSupplierTariffsTx(tx, &state, profile, contracts, hash)
		if err != nil {
			return err
		}
		return tx.Model(&SupplierPricingState{}).Where("profile_id = ?", profileID).Updates(map[string]any{"candidate_hash": "", "confirmations": 0}).Error
	})
	return revision, err
}

// ConfirmSupplierProfileTariffs requires two matching complete fetches. Missing
// models remain in the old revision until the replacement catalogue is confirmed.
func ConfirmSupplierProfileTariffs(profileID int, expectedConfigRevision, expectedRevision int64, tariffs []types.SupplierModelTariff) (int64, bool, error) {
	contracts, hash, err := supplierTariffCandidate(tariffs)
	if err != nil {
		return 0, false, err
	}
	var revision int64
	pending := true
	err = DB.Transaction(func(tx *gorm.DB) error {
		profile, err := supplierPricingProfileTx(tx, profileID, expectedConfigRevision)
		if err != nil {
			return err
		}
		if !profile.Enabled {
			return errors.New("supplier pricing profile is disabled")
		}
		state, err := supplierPricingStateTx(tx, profileID)
		if err != nil {
			return err
		}
		if state.ActiveRevision != expectedRevision {
			return ErrSupplierPricingConflict
		}
		revision = state.ActiveRevision
		confirmations := 1
		if state.CandidateHash == hash && state.CandidateConfigRevision == expectedConfigRevision {
			confirmations = state.Confirmations + 1
		}
		if confirmations >= 2 {
			revision, err = publishSupplierTariffsTx(tx, &state, profile, contracts, hash)
			if err != nil {
				return err
			}
			pending = false
		}
		return tx.Model(&SupplierPricingState{}).Where("profile_id = ?", profileID).Updates(map[string]any{"candidate_hash": hash, "candidate_config_revision": expectedConfigRevision, "confirmations": confirmations}).Error
	})
	return revision, pending, err
}

func ListSupplierProfileTariffs(profileID int) ([]types.SupplierModelTariff, int64, error) {
	state := SupplierPricingState{}
	err := DB.First(&state, "profile_id = ?", profileID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []types.SupplierModelTariff{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows := make([]SupplierModelPrice, 0)
	if err := DB.Where("profile_id = ? AND revision = ?", profileID, state.ActiveRevision).Order("model_name ASC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	tariffs := make([]types.SupplierModelTariff, 0, len(rows))
	for _, row := range rows {
		tariff := types.SupplierModelTariff{}
		if err := common.UnmarshalJsonStr(row.Data, &tariff); err != nil {
			return nil, 0, err
		}
		tariff.ProfileID, tariff.Revision = profileID, state.ActiveRevision
		tariffs = append(tariffs, tariff)
	}
	return tariffs, state.ActiveRevision, nil
}

func ResolveSupplierModelTariff(group, modelName string) (types.SupplierModelTariff, bool, error) {
	zero := types.SupplierModelTariff{}
	binding, err := GetPricingGroupSupplierBinding(group)
	if err != nil {
		return zero, true, ErrSupplierTariffUnavailable
	}
	if binding.SupplierID == 0 && binding.ProfileID == 0 {
		return zero, false, nil
	}
	var row struct {
		Revision int64
		Data     string
	}
	// One statement checks enablement, configuration and the active revision
	// together; an intervening profile edit cannot mix old and new settings.
	err = DB.Table("supplier_pricing_profiles AS profiles").
		Select("prices.revision, prices.data").
		Joins("JOIN suppliers AS suppliers ON suppliers.id = profiles.supplier_id").
		Joins("JOIN supplier_pricing_states AS states ON states.profile_id = profiles.id").
		Joins("JOIN supplier_pricing_revisions AS revisions ON revisions.profile_id = profiles.id AND revisions.revision = states.active_revision AND revisions.config_revision = profiles.config_revision").
		Joins("JOIN supplier_model_prices AS prices ON prices.profile_id = profiles.id AND prices.revision = states.active_revision").
		Where("profiles.id = ? AND profiles.supplier_id = ? AND profiles.enabled = ? AND suppliers.enabled = ? AND prices.model_name = ?", binding.ProfileID, binding.SupplierID, true, true, modelName).
		Take(&row).Error
	if err != nil {
		return zero, true, ErrSupplierTariffUnavailable
	}
	tariff := types.SupplierModelTariff{}
	if err := common.UnmarshalJsonStr(row.Data, &tariff); err != nil {
		return zero, true, ErrSupplierTariffUnavailable
	}
	if tariff.ModelName != modelName {
		return zero, true, ErrSupplierTariffUnavailable
	}
	tariff.SupplierID, tariff.ProfileID, tariff.Revision = binding.SupplierID, binding.ProfileID, row.Revision
	if err := ValidateSupplierModelTariff(tariff); err != nil {
		return zero, true, ErrSupplierTariffUnavailable
	}
	return tariff, true, nil
}
