package model

import (
	"database/sql"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const publicPricingQueryBatch = 900

type publicPricingProfile struct {
	supplierID int
	tariffs    map[string]types.SupplierModelTariff
}

type publicPricingData struct {
	abilities []AbilityWithChannel
	targets   []string
	channels  map[int]Channel
	metadata  []Model
	official  map[string]OfficialAPIPrice
	bindings  map[int]PricingGroupSupplierBinding
	profiles  map[int]publicPricingProfile
}

// loadPublicPricingData reads one coherent procurement snapshot. None of the
// per-model rendering below it performs queries or uses the runtime resolver.
func loadPublicPricingData(usable map[string]string) (publicPricingData, error) {
	data := publicPricingData{
		channels: make(map[int]Channel), official: make(map[string]OfficialAPIPrice),
		bindings: make(map[int]PricingGroupSupplierBinding), profiles: make(map[int]publicPricingProfile),
	}
	isolation := sql.LevelRepeatableRead
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		isolation = sql.LevelSerializable
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("abilities").
			Select("abilities.*, channels.type as channel_type, channels.model_mapping as channel_model_mapping").
			Joins("left join channels on abilities.channel_id = channels.id").
			Where("abilities.enabled = ?", true).Scan(&data.abilities).Error; err != nil {
			return err
		}
		if len(data.abilities) == 0 {
			return nil
		}
		var channels []Channel
		channelColumns := []string{"id", "status"}
		if channelSupplierColumnsReadyTx(tx) {
			channelColumns = append(channelColumns, "supplier_id", "supplier_profile_id")
		}
		if err := tx.Select(channelColumns).
			Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			data.channels[channel.Id] = channel
		}
		if err := tx.Find(&data.metadata).Error; err != nil {
			return err
		}
		groupIDs := make([]int, 0, len(usable))
		for group := range usable {
			if ref, ok := ratio_setting.PricingGroupRefByKey(group); ok {
				groupIDs = append(groupIDs, ref.Id)
			}
		}
		slices.Sort(groupIDs)
		groupIDs = slices.Compact(groupIDs)
		bindingsReady := tx.Migrator().HasTable(&PricingGroupSupplierBinding{})
		for start := 0; bindingsReady && start < len(groupIDs); start += publicPricingQueryBatch {
			var bindings []PricingGroupSupplierBinding
			if err := tx.Where("group_id IN ?", groupIDs[start:min(start+publicPricingQueryBatch, len(groupIDs))]).Find(&bindings).Error; err != nil {
				return err
			}
			for _, binding := range bindings {
				data.bindings[binding.GroupID] = binding
			}
		}
		names := make([]string, 0, len(data.abilities))
		billingNames := make([]string, 0, len(data.abilities))
		data.targets = make([]string, len(data.abilities))
		for index, ability := range data.abilities {
			names = append(names, ability.Model)
			group := ratio_setting.PricingGroupKey(ability.Group)
			if _, ok := usable[group]; !ok {
				continue
			}
			target, mapped := resolveChannelModelPricingTarget(ability.Model, ability.ChannelModelMapping)
			if !mapped {
				target = ability.Model
			}
			data.targets[index] = target
			if target != "" {
				billingNames = append(billingNames, target)
			}
		}
		slices.Sort(billingNames)
		billingNames = slices.Compact(billingNames)
		if err := data.loadProfiles(tx, billingNames); err != nil {
			return err
		}
		slices.Sort(names)
		names = slices.Compact(names)
		officialPricesReady := tx.Migrator().HasTable(&OfficialModelPrice{})
		for start := 0; officialPricesReady && start < len(names); start += publicPricingQueryBatch {
			var prices []OfficialModelPrice
			if err := tx.Where("model_name IN ?", names[start:min(start+publicPricingQueryBatch, len(names))]).Find(&prices).Error; err != nil {
				return err
			}
			for _, price := range prices {
				if ValidateOfficialModelPrice(price) == nil {
					data.official[price.ModelName] = OfficialAPIPrice{
						InputUSD: *price.InputUSD, OutputUSD: *price.OutputUSD,
						Source: price.Source, UpdatedAt: price.UpdatedAt, PriceUnit: price.PriceUnit,
					}
				}
			}
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: isolation})
	return data, err
}

func (data publicPricingData) loadProfiles(tx *gorm.DB, names []string) error {
	ids := make([]int, 0, len(data.bindings))
	for _, binding := range data.bindings {
		if binding.ProfileID > 0 {
			ids = append(ids, binding.ProfileID)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil
	}
	for _, table := range []any{&Supplier{}, &SupplierPricingProfile{}, &SupplierPricingState{}, &SupplierPricingRevision{}, &SupplierModelPrice{}} {
		if !tx.Migrator().HasTable(table) {
			return nil
		}
	}
	// Two IN clauses share the parameter budget. Fetch only model contracts
	// reachable from the catalogue, not an entire upstream profile catalogue.
	const batch = publicPricingQueryBatch / 2
	for start := 0; start < len(ids); start += batch {
		for nameStart := 0; nameStart < max(1, len(names)); nameStart += batch {
			var rows []struct {
				ProfileID  int
				SupplierID int
				Revision   int64
				ModelName  *string
				Data       *string
			}
			// Left joins retain enabled profiles without a published current revision:
			// their models stay unavailable rather than borrowing default prices.
			if err := tx.Table("supplier_pricing_profiles AS profiles").
				Select("profiles.id AS profile_id, profiles.supplier_id, COALESCE(revisions.revision, 0) AS revision, prices.model_name, prices.data").
				Joins("JOIN suppliers ON suppliers.id = profiles.supplier_id").
				Joins("LEFT JOIN supplier_pricing_states AS states ON states.profile_id = profiles.id").
				Joins("LEFT JOIN supplier_pricing_revisions AS revisions ON revisions.profile_id = profiles.id AND revisions.revision = states.active_revision AND revisions.config_revision = profiles.config_revision").
				Joins("LEFT JOIN supplier_model_prices AS prices ON prices.profile_id = profiles.id AND prices.revision = revisions.revision AND prices.model_name IN ?", names[nameStart:min(nameStart+batch, len(names))]).
				Where("profiles.id IN ? AND profiles.enabled = ? AND suppliers.enabled = ?", ids[start:min(start+batch, len(ids))], true, true).
				Scan(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				profile, exists := data.profiles[row.ProfileID]
				if !exists {
					profile = publicPricingProfile{supplierID: row.SupplierID, tariffs: make(map[string]types.SupplierModelTariff)}
					data.profiles[row.ProfileID] = profile
				}
				if row.ModelName == nil || row.Data == nil {
					continue
				}
				var tariff types.SupplierModelTariff
				if common.UnmarshalJsonStr(*row.Data, &tariff) != nil || tariff.ModelName != *row.ModelName || ValidateSupplierModelTariff(tariff) != nil {
					continue
				}
				tariff.SupplierID, tariff.ProfileID, tariff.Revision = row.SupplierID, row.ProfileID, row.Revision
				profile.tariffs[tariff.ModelName] = tariff
			}
		}
	}
	return nil
}

func (data publicPricingData) channelMatches(channel Channel, binding PricingGroupSupplierBinding) bool {
	if binding.SupplierID == 0 && binding.ProfileID == 0 {
		return channel.SupplierID == nil && channel.SupplierProfileID == nil
	}
	if channel.SupplierID == nil || channel.SupplierProfileID == nil || *channel.SupplierID != binding.SupplierID || *channel.SupplierProfileID != binding.ProfileID {
		return false
	}
	profile, exists := data.profiles[binding.ProfileID]
	return exists && profile.supplierID == binding.SupplierID
}
