package model

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type SupplierMigrationChannel struct {
	ChannelID          int      `json:"channel_id"`
	Name               string   `json:"name"`
	Tag                string   `json:"tag"`
	GroupIDs           []int    `json:"group_ids"`
	SupplierID         *int     `json:"supplier_id"`
	ProfileID          *int     `json:"profile_id"`
	ProposedSupplierID *int     `json:"proposed_supplier_id"`
	ProposedProfileID  *int     `json:"proposed_profile_id"`
	SourceIDs          []int    `json:"source_ids"`
	Warnings           []string `json:"warnings"`
}

type SupplierMigrationGroup struct {
	GroupID    int      `json:"group_id"`
	Name       string   `json:"name"`
	ChannelIDs []int    `json:"channel_ids"`
	Tags       []string `json:"tags"`
	Warnings   []string `json:"warnings"`
}

type SupplierMigrationPreview struct {
	Channels                  []SupplierMigrationChannel `json:"channels"`
	Groups                    []SupplierMigrationGroup   `json:"groups"`
	DefaultSourceIDs          []int                      `json:"default_source_ids"`
	Warnings                  []string                   `json:"warnings"`
	AutomaticMigrationAllowed bool                       `json:"automatic_migration_allowed"`
}

// GetSupplierMigrationPreview never changes tags, prices or active bindings.
func GetSupplierMigrationPreview() (SupplierMigrationPreview, error) {
	preview := SupplierMigrationPreview{
		Channels:         make([]SupplierMigrationChannel, 0),
		Groups:           make([]SupplierMigrationGroup, 0),
		DefaultSourceIDs: make([]int, 0),
		Warnings:         []string{"tariff_equivalence_not_verified", "existing_tags_and_global_prices_are_preserved", "explicit_binding_review_required"},
	}
	suppliers, err := ListSuppliers()
	if err != nil {
		return preview, err
	}
	profiles, err := ListSupplierPricingProfiles(0)
	if err != nil {
		return preview, err
	}
	var channels []Channel
	if err := DB.Select("id", "name", "tag", "group", "supplier_id", "supplier_profile_id").Order("id").Find(&channels).Error; err != nil {
		return preview, err
	}
	var sources []PricingSyncSource
	if err := DB.Select("channel_id").Find(&sources).Error; err != nil {
		return preview, err
	}
	sourceIDs := make(map[int]bool)
	for _, source := range sources {
		if source.ChannelID > 0 {
			sourceIDs[source.ChannelID] = true
		} else {
			preview.DefaultSourceIDs = append(preview.DefaultSourceIDs, source.ChannelID)
		}
	}
	profilesBySupplier := make(map[int][]SupplierPricingProfile)
	for _, profile := range profiles {
		if profile.Enabled {
			profilesBySupplier[profile.SupplierID] = append(profilesBySupplier[profile.SupplierID], profile)
		}
	}
	groups := ratio_setting.GetPricingGroupsCopy()
	groupIndex := make(map[int]int)
	groupTags := make(map[int]map[string]bool)
	for _, group := range groups {
		groupIndex[group.Id] = len(preview.Groups)
		groupTags[group.Id] = make(map[string]bool)
		preview.Groups = append(preview.Groups, SupplierMigrationGroup{GroupID: group.Id, Name: group.Name, ChannelIDs: make([]int, 0), Tags: make([]string, 0), Warnings: make([]string, 0)})
	}
	for _, channel := range channels {
		item := SupplierMigrationChannel{ChannelID: channel.Id, Name: channel.Name, Tag: channel.GetTag(), SupplierID: channel.SupplierID, ProfileID: channel.SupplierProfileID, GroupIDs: make([]int, 0), SourceIDs: make([]int, 0), Warnings: make([]string, 0)}
		if sourceIDs[channel.Id] {
			item.SourceIDs = append(item.SourceIDs, channel.Id)
		}
		for _, key := range channel.GetGroups() {
			if group, ok := ratio_setting.ResolvePricingGroupKey(key); ok {
				item.GroupIDs = append(item.GroupIDs, group.Id)
				index := groupIndex[group.Id]
				preview.Groups[index].ChannelIDs = append(preview.Groups[index].ChannelIDs, channel.Id)
				groupTags[group.Id][item.Tag] = true
			} else {
				item.Warnings = append(item.Warnings, "unresolved_legacy_group")
			}
		}
		if channel.SupplierID == nil {
			matches := make([]Supplier, 0)
			for _, supplier := range suppliers {
				if supplier.Enabled && item.Tag != "" && (strings.EqualFold(item.Tag, supplier.Code) || strings.EqualFold(item.Tag, supplier.Name)) {
					matches = append(matches, supplier)
				}
			}
			if len(matches) == 1 && len(profilesBySupplier[matches[0].ID]) == 1 {
				supplierID, profileID := matches[0].ID, profilesBySupplier[matches[0].ID][0].ID
				item.ProposedSupplierID, item.ProposedProfileID = &supplierID, &profileID
				item.Warnings = append(item.Warnings, "tariff_equivalence_not_verified")
			} else if len(matches) > 0 {
				item.Warnings = append(item.Warnings, "ambiguous_supplier_or_profile")
			} else {
				item.Warnings = append(item.Warnings, "no_matching_supplier")
			}
		}
		preview.Channels = append(preview.Channels, item)
	}
	for i := range preview.Groups {
		group := &preview.Groups[i]
		for tag := range groupTags[group.GroupID] {
			group.Tags = append(group.Tags, tag)
		}
		sort.Strings(group.Tags)
		if len(group.Tags) > 1 {
			group.Warnings = append(group.Warnings, "mixed_channel_tags")
		}
		if len(group.ChannelIDs) > 0 {
			group.Warnings = append(group.Warnings, "all_group_channels_require_atomic_review")
		}
	}
	sort.Ints(preview.DefaultSourceIDs)
	return preview, nil
}
