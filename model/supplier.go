package model

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Supplier is an administrative procurement identity, not a model developer.
type Supplier struct {
	ID      int    `json:"id" gorm:"primaryKey"`
	Code    string `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Name    string `json:"name" gorm:"size:128;not null"`
	Adapter string `json:"adapter" gorm:"size:32;not null"`
	Enabled bool   `json:"enabled"`
}

type SupplierPricingProfile struct {
	ID              int     `json:"id" gorm:"primaryKey"`
	SupplierID      int     `json:"supplier_id" gorm:"uniqueIndex:idx_supplier_profile_code;index;not null"`
	Code            string  `json:"code" gorm:"size:64;uniqueIndex:idx_supplier_profile_code;not null"`
	Name            string  `json:"name" gorm:"size:128;not null"`
	Enabled         bool    `json:"enabled"`
	BaseURL         string  `json:"base_url" gorm:"size:512"`
	Endpoint        string  `json:"endpoint" gorm:"size:512"`
	UpstreamGroup   string  `json:"upstream_group" gorm:"size:128"`
	USDPerCredit    float64 `json:"usd_per_credit"`
	IntervalSeconds int64   `json:"interval_seconds"`
	ConfigRevision  int64   `json:"config_revision" gorm:"not null;default:1"`
	LastAttemptAt   int64   `json:"last_attempt_at"`
	LastSuccessAt   int64   `json:"last_success_at"`
	LastError       string  `json:"last_error" gorm:"type:text"`
}

// PricingGroupSupplierBinding stays outside the public PricingGroups option.
type PricingGroupSupplierBinding struct {
	GroupID    int `json:"group_id" gorm:"primaryKey;autoIncrement:false"`
	SupplierID int `json:"supplier_id" gorm:"index;not null"`
	ProfileID  int `json:"profile_id" gorm:"index;not null"`
}

// SupplierBindingState serializes changes even when a binding does not yet exist.
type SupplierBindingState struct {
	ID       int   `gorm:"primaryKey;autoIncrement:false"`
	Revision int64 `gorm:"not null;default:0"`
}

type SupplierGroupBindingPatch struct {
	GroupID    int  `json:"group_id"`
	SupplierID *int `json:"supplier_id"`
	ProfileID  *int `json:"profile_id"`
}

type SupplierChannelBindingPatch struct {
	ChannelID  int     `json:"channel_id"`
	SupplierID *int    `json:"supplier_id"`
	ProfileID  *int    `json:"profile_id"`
	Group      *string `json:"group,omitempty"`
}

var supplierCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

func lockSupplierBindingsTx(tx *gorm.DB) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&SupplierBindingState{ID: 1}).Error; err != nil {
		return err
	}
	return tx.Model(&SupplierBindingState{}).Where("id = ?", 1).UpdateColumn("revision", gorm.Expr("revision + 1")).Error
}

func GetSupplier(id int) (Supplier, error) {
	var supplier Supplier
	err := DB.First(&supplier, "id = ?", id).Error
	return supplier, err
}

func ListSuppliers() ([]Supplier, error) {
	suppliers := make([]Supplier, 0)
	err := DB.Order("id").Find(&suppliers).Error
	return suppliers, err
}

func validateSupplier(supplier *Supplier) error {
	supplier.Code = strings.TrimSpace(supplier.Code)
	supplier.Name = strings.TrimSpace(supplier.Name)
	supplier.Adapter = strings.TrimSpace(supplier.Adapter)
	if !supplierCodePattern.MatchString(supplier.Code) || supplier.Name == "" || len(supplier.Name) > 128 {
		return errors.New("invalid supplier code or name")
	}
	if supplier.Adapter == "" || len(supplier.Adapter) > 32 || !supplierCodePattern.MatchString(supplier.Adapter) {
		return errors.New("invalid supplier adapter")
	}
	switch supplier.Adapter {
	case "new-api", "openlux", "bblabu", "relay-fast", "manual":
	default:
		return errors.New("unsupported supplier adapter")
	}
	return nil
}

func CreateSupplier(supplier *Supplier) error {
	if err := validateSupplier(supplier); err != nil {
		return err
	}
	supplier.ID = 0
	return DB.Create(supplier).Error
}

func UpdateSupplier(supplier *Supplier) error {
	if err := validateSupplier(supplier); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		var current Supplier
		if err := tx.First(&current, "id = ?", supplier.ID).Error; err != nil {
			return err
		}
		if current.Adapter != supplier.Adapter {
			var count int64
			if err := tx.Model(&SupplierPricingProfile{}).Where("supplier_id = ?", supplier.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("supplier adapter cannot change while profiles exist")
			}
		}
		return tx.Model(&current).Select("code", "name", "adapter", "enabled").Updates(supplier).Error
	})
}

func DeleteSupplier(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		for _, item := range []any{&SupplierPricingProfile{}, &Channel{}, &PricingGroupSupplierBinding{}} {
			var count int64
			if err := tx.Model(item).Where("supplier_id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("supplier is still in use")
			}
		}
		result := tx.Delete(&Supplier{}, "id = ?", id)
		if result.Error == nil && result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return result.Error
	})
}

func GetSupplierPricingProfile(id int) (SupplierPricingProfile, error) {
	var profile SupplierPricingProfile
	err := DB.First(&profile, "id = ?", id).Error
	return profile, err
}

func ListSupplierPricingProfiles(supplierID int) ([]SupplierPricingProfile, error) {
	profiles := make([]SupplierPricingProfile, 0)
	query := DB.Order("id")
	if supplierID > 0 {
		query = query.Where("supplier_id = ?", supplierID)
	}
	err := query.Find(&profiles).Error
	return profiles, err
}

func validateSupplierPricingProfile(profile *SupplierPricingProfile) error {
	profile.Code = strings.TrimSpace(profile.Code)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.BaseURL = strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	profile.Endpoint = strings.TrimSpace(profile.Endpoint)
	profile.UpstreamGroup = strings.TrimSpace(profile.UpstreamGroup)
	if !supplierCodePattern.MatchString(profile.Code) || profile.Name == "" || len(profile.Name) > 128 || len(profile.UpstreamGroup) > 128 {
		return errors.New("invalid supplier profile code or name")
	}
	if profile.SupplierID <= 0 || math.IsNaN(profile.USDPerCredit) || math.IsInf(profile.USDPerCredit, 0) || profile.USDPerCredit <= 0 {
		return errors.New("supplier profile requires a positive USD per credit conversion")
	}
	if profile.IntervalSeconds != 0 && (profile.IntervalSeconds < 60 || profile.IntervalSeconds > 604800) {
		return errors.New("supplier sync interval must be zero or between 60 and 604800 seconds")
	}
	if profile.BaseURL == "" {
		if profile.Endpoint != "" {
			return errors.New("supplier profile endpoint requires a base URL")
		}
		return nil
	}
	baseURL, err := url.Parse(profile.BaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Hostname() == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || len(profile.BaseURL) > 512 {
		return errors.New("supplier profile base URL must be HTTPS without credentials, query or fragment")
	}
	endpoint, err := url.Parse(profile.Endpoint)
	if err != nil || endpoint.IsAbs() || endpoint.Host != "" || endpoint.User != nil || endpoint.Fragment != "" || !strings.HasPrefix(profile.Endpoint, "/") || strings.HasPrefix(profile.Endpoint, "//") || len(profile.Endpoint) > 512 {
		return errors.New("supplier profile endpoint must be a relative absolute path")
	}
	return nil
}

func CreateSupplierPricingProfile(profile *SupplierPricingProfile) error {
	profile.ID = 0
	profile.ConfigRevision = 1
	profile.LastAttemptAt, profile.LastSuccessAt, profile.LastError = 0, 0, ""
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		var supplier Supplier
		if err := tx.First(&supplier, "id = ?", profile.SupplierID).Error; err != nil {
			return err
		}
		if profile.Endpoint == "" && profile.BaseURL != "" {
			switch supplier.Adapter {
			case "openlux", "bblabu":
				profile.Endpoint = "/api/pricing"
			case "relay-fast":
				profile.Endpoint = "/pricing.json"
			}
		}
		if err := validateSupplierPricingProfile(profile); err != nil {
			return err
		}
		return tx.Create(profile).Error
	})
}

func UpdateSupplierPricingProfile(profile *SupplierPricingProfile) error {
	if err := validateSupplierPricingProfile(profile); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		var current SupplierPricingProfile
		if err := tx.First(&current, "id = ?", profile.ID).Error; err != nil {
			return err
		}
		if profile.SupplierID != current.SupplierID {
			return errors.New("supplier profile cannot move to another supplier")
		}
		if profile.ConfigRevision != current.ConfigRevision {
			return errors.New("supplier profile configuration changed; reload before saving")
		}
		profile.ConfigRevision++
		profile.LastAttemptAt, profile.LastSuccessAt, profile.LastError = current.LastAttemptAt, current.LastSuccessAt, current.LastError
		return tx.Model(&current).Select("code", "name", "enabled", "base_url", "endpoint", "upstream_group", "usd_per_credit", "interval_seconds", "config_revision").Updates(profile).Error
	})
}

func DeleteSupplierPricingProfile(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		var profile SupplierPricingProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, "id = ?", id).Error; err != nil {
			return err
		}
		for _, item := range []struct {
			model any
			field string
		}{{&Channel{}, "supplier_profile_id"}, {&PricingGroupSupplierBinding{}, "profile_id"}, {&SupplierPricingRevision{}, "profile_id"}} {
			var count int64
			if err := tx.Model(item.model).Where(item.field+" = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("supplier profile is still in use")
			}
		}
		if err := tx.Delete(&SupplierPricingState{}, "profile_id = ?", id).Error; err != nil {
			return err
		}
		result := tx.Delete(&SupplierPricingProfile{}, "id = ?", id)
		if result.Error == nil && result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return result.Error
	})
}

func validateSupplierPairTx(tx *gorm.DB, supplierID, profileID *int) error {
	if supplierID == nil && profileID == nil {
		return nil
	}
	if supplierID == nil || profileID == nil || *supplierID <= 0 || *profileID <= 0 {
		return errors.New("supplier and pricing profile must be selected together")
	}
	var supplier Supplier
	if err := tx.First(&supplier, "id = ? AND enabled = ?", *supplierID, true).Error; err != nil {
		return errors.New("supplier is unavailable")
	}
	var profile SupplierPricingProfile
	if err := tx.First(&profile, "id = ? AND supplier_id = ? AND enabled = ?", *profileID, *supplierID, true).Error; err != nil {
		return errors.New("supplier pricing profile is unavailable")
	}
	return nil
}

func getPricingGroupSupplierBindingTx(tx *gorm.DB, group string) (PricingGroupSupplierBinding, error) {
	if tx == nil {
		return PricingGroupSupplierBinding{}, errors.New("pricing database is unavailable")
	}
	if !tx.Migrator().HasTable(&PricingGroupSupplierBinding{}) {
		// An expanded schema is optional until its startup migration has run.
		// A broken database must not be mistaken for an unbound group.
		var available int
		if err := tx.Raw("SELECT 1").Scan(&available).Error; err != nil {
			return PricingGroupSupplierBinding{}, err
		}
		return PricingGroupSupplierBinding{}, nil
	}
	resolved, ok := ratio_setting.ResolvePricingGroupKey(group)
	if !ok {
		return PricingGroupSupplierBinding{}, errors.New("pricing group is unavailable")
	}
	var binding PricingGroupSupplierBinding
	err := tx.Where("group_id = ?", resolved.Id).Limit(1).Find(&binding).Error
	return binding, err
}

func GetPricingGroupSupplierBinding(group string) (PricingGroupSupplierBinding, error) {
	return getPricingGroupSupplierBindingTx(DB, group)
}

func validateChannelPricingGroupTx(tx *gorm.DB, channel *Channel, group string) error {
	if err := validateSupplierPairTx(tx, channel.SupplierID, channel.SupplierProfileID); err != nil {
		return err
	}
	if _, ok := ratio_setting.ResolvePricingGroupKey(group); !ok && channel.SupplierID == nil {
		// Legacy unbound groups keep their existing routing semantics.
		return nil
	}
	binding, err := getPricingGroupSupplierBindingTx(tx, group)
	if err != nil {
		return err
	}
	if binding.SupplierID == 0 && channel.SupplierID == nil {
		return nil
	}
	if channel.SupplierID == nil || channel.SupplierProfileID == nil || binding.SupplierID != *channel.SupplierID || binding.ProfileID != *channel.SupplierProfileID {
		return errors.New("channel and pricing group must use the same supplier pricing profile")
	}
	return nil
}

func ValidateChannelPricingGroup(channel *Channel, group string) error {
	if err := validateChannelPricingGroupTx(DB, channel, group); err != nil {
		// This helper is used in request routing; keep procurement details private.
		return errors.New("channel is unavailable for the selected pricing group")
	}
	return nil
}

func validateChannelSupplierGroupsTx(tx *gorm.DB, channel *Channel) error {
	groups := channel.GetGroups()
	if len(groups) == 0 && (channel.SupplierID != nil || channel.SupplierProfileID != nil) {
		return errors.New("supplier channel must belong to a pricing group")
	}
	for _, group := range groups {
		if err := validateChannelPricingGroupTx(tx, channel, group); err != nil {
			return err
		}
	}
	return nil
}

func lockChannelSupplierMutationTx(tx *gorm.DB) error {
	// Existing installations and isolated model tests may precede this migration.
	if !tx.Migrator().HasTable(&PricingGroupSupplierBinding{}) {
		return nil
	}
	return lockSupplierBindingsTx(tx)
}

func validateChannelSupplierMutationTx(tx *gorm.DB, channel *Channel) error {
	if !tx.Migrator().HasTable(&PricingGroupSupplierBinding{}) {
		if channel.SupplierID != nil || channel.SupplierProfileID != nil {
			return errors.New("supplier bindings are not available before database migration")
		}
		return nil
	}
	return validateChannelSupplierGroupsTx(tx, channel)
}

func prepareChannelSupplierMutationTx(tx *gorm.DB, channel *Channel, allowSupplierBindingChange bool) error {
	if err := lockChannelSupplierMutationTx(tx); err != nil {
		return err
	}
	if channel.Id > 0 {
		var current Channel
		err := tx.First(&current, "id = ?", channel.Id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			pairChanged := !sameSupplierPointer(current.SupplierID, channel.SupplierID) || !sameSupplierPointer(current.SupplierProfileID, channel.SupplierProfileID)
			groupsChanged := current.Group != channel.Group
			transportChanged := channel.Type != 0 && channel.Type != current.Type ||
				channel.Key != "" && channel.Key != current.Key ||
				channel.BaseURL != nil && channel.GetBaseURL() != current.GetBaseURL()
			if !allowSupplierBindingChange && (pairChanged || groupsChanged && (current.SupplierID != nil || channel.SupplierID != nil)) {
				return errors.New("insufficient permissions to change supplier channel bindings")
			}
			if pairChanged || transportChanged {
				if err := ensureSupplierChannelIdleTx(tx, channel.Id); err != nil {
					return err
				}
			}
			if !pairChanged && !groupsChanged {
				return nil
			}
		}
	}
	return validateChannelSupplierMutationTx(tx, channel)
}

func ensureSupplierChannelIdleTx(tx *gorm.DB, channelID int) error {
	var pending int64
	if tx.Migrator().HasTable(&Midjourney{}) {
		if err := tx.Model(&Midjourney{}).Where("channel_id = ? AND (progress IS NULL OR progress != ?)", channelID, "100%").Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return errors.New("channel connection cannot change while legacy image tasks are pending")
		}
	}
	if tx.Migrator().HasTable(&Task{}) {
		if err := tx.Model(&Task{}).Where("channel_id = ? AND (status IS NULL OR status NOT IN ?)", channelID, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return errors.New("channel connection cannot change while asynchronous tasks are pending")
		}
	}
	return nil
}

func sameSupplierPointer(a, b *int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func ListPricingGroupSupplierBindings() ([]PricingGroupSupplierBinding, error) {
	bindings := make([]PricingGroupSupplierBinding, 0)
	err := DB.Order("group_id").Find(&bindings).Error
	return bindings, err
}

func ApplySupplierBindings(bindings []SupplierGroupBindingPatch, channels []SupplierChannelBindingPatch) error {
	if len(bindings) == 0 && len(channels) == 0 {
		return errors.New("no supplier bindings supplied")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSupplierBindingsTx(tx); err != nil {
			return err
		}
		groupIDs, err := supplierPricingGroupIDsTx(tx)
		if err != nil {
			return err
		}
		seenGroups, seenChannels := make(map[int]bool), make(map[int]bool)
		for _, patch := range bindings {
			if !groupIDs[patch.GroupID] || seenGroups[patch.GroupID] {
				return errors.New("invalid or duplicate pricing group")
			}
			seenGroups[patch.GroupID] = true
			if err := validateSupplierPairTx(tx, patch.SupplierID, patch.ProfileID); err != nil {
				return err
			}
			if patch.SupplierID == nil {
				if err := tx.Delete(&PricingGroupSupplierBinding{}, "group_id = ?", patch.GroupID).Error; err != nil {
					return err
				}
			} else {
				binding := PricingGroupSupplierBinding{GroupID: patch.GroupID, SupplierID: *patch.SupplierID, ProfileID: *patch.ProfileID}
				if err := tx.Save(&binding).Error; err != nil {
					return err
				}
			}
		}
		for _, patch := range channels {
			if patch.ChannelID <= 0 || seenChannels[patch.ChannelID] {
				return errors.New("invalid or duplicate channel")
			}
			seenChannels[patch.ChannelID] = true
			if err := validateSupplierPairTx(tx, patch.SupplierID, patch.ProfileID); err != nil {
				return err
			}
			var current Channel
			if err := tx.First(&current, "id = ?", patch.ChannelID).Error; err != nil {
				return err
			}
			if !sameSupplierPointer(current.SupplierID, patch.SupplierID) || !sameSupplierPointer(current.SupplierProfileID, patch.ProfileID) {
				if err := ensureSupplierChannelIdleTx(tx, patch.ChannelID); err != nil {
					return err
				}
			}
			updates := map[string]any{"supplier_id": patch.SupplierID, "supplier_profile_id": patch.ProfileID}
			if patch.Group != nil {
				group := ratio_setting.PricingGroupKeysCSV(*patch.Group)
				if group == "" {
					return errors.New("channel must belong to a pricing group")
				}
				updates["group"] = group
			}
			result := tx.Model(&Channel{}).Where("id = ?", patch.ChannelID).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				var existing Channel
				if err := tx.First(&existing, "id = ?", patch.ChannelID).Error; err != nil {
					return err
				}
			}
		}
		var allChannels []Channel
		if err := tx.Find(&allChannels).Error; err != nil {
			return err
		}
		for i := range allChannels {
			affected := seenChannels[allChannels[i].Id]
			for _, group := range allChannels[i].GetGroups() {
				if resolved, ok := ratio_setting.ResolvePricingGroupKey(group); ok && seenGroups[resolved.Id] {
					affected = true
				}
			}
			if affected {
				if err := validateChannelSupplierGroupsTx(tx, &allChannels[i]); err != nil {
					return fmt.Errorf("channel %d: %w", allChannels[i].Id, err)
				}
				if seenChannels[allChannels[i].Id] {
					if err := allChannels[i].UpdateAbilities(tx); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

func supplierPricingGroupIDsTx(tx *gorm.DB) (map[int]bool, error) {
	groups := ratio_setting.GetPricingGroupsCopy()
	var option Option
	err := tx.Where(map[string]any{"key": "PricingGroups"}).First(&option).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && option.Value != "" {
		normalized, err := ratio_setting.NormalizePricingGroupsJSONString(option.Value)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal([]byte(normalized), &groups); err != nil {
			return nil, err
		}
	}
	ids := make(map[int]bool, len(groups))
	for _, group := range groups {
		ids[group.Id] = true
	}
	return ids, nil
}

func ValidateSupplierPricingGroupsJSON(value string) error {
	if DB == nil || !DB.Migrator().HasTable(&PricingGroupSupplierBinding{}) {
		return nil
	}
	return validateSupplierPricingGroupsJSONTx(DB, value)
}

func validateSupplierPricingGroupsJSONTx(tx *gorm.DB, value string) error {
	normalized, err := ratio_setting.NormalizePricingGroupsJSONString(value)
	if err != nil {
		return err
	}
	var groups []ratio_setting.PricingGroup
	if err := common.Unmarshal([]byte(normalized), &groups); err != nil {
		return err
	}
	ids := make(map[int]bool, len(groups))
	for _, group := range groups {
		ids[group.Id] = true
	}
	var bindings []PricingGroupSupplierBinding
	if err := tx.Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		if !ids[binding.GroupID] {
			return errors.New("cannot remove a pricing group with a supplier binding")
		}
	}
	return nil
}
