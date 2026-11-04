package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSupplierLegacyChannelTest(t *testing.T, partial bool) {
	t.Helper()
	previousDB := DB
	previousGroups := ratio_setting.PricingGroups2JSONString()
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		initCol()
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true}]`))
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
	columns := []string{"supplier_profile_id"}
	if !partial {
		columns = append(columns, "supplier_id")
	}
	for _, column := range columns {
		if DB.Migrator().HasIndex(&Channel{}, "idx_channels_"+column) {
			require.NoError(t, DB.Migrator().DropIndex(&Channel{}, "idx_channels_"+column))
		}
		require.NoError(t, DB.Migrator().DropColumn(&Channel{}, column))
	}
}

func TestSupplierUnboundChannelWritesBeforeMigration(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial_%t", partial), func(t *testing.T) {
			setupSupplierLegacyChannelTest(t, partial)
			channel := Channel{Name: "created", Key: "legacy-key", Models: "model", Group: "default"}
			require.NoError(t, channel.Insert())
			require.Equal(t, "1", channel.Group)
			loaded, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.Nil(t, loaded.SupplierID)
			require.Nil(t, loaded.SupplierProfileID)
			loaded.Name = "saved"
			require.NoError(t, loaded.Save())
			loaded.Key, loaded.Name = "", "saved-without-key"
			require.NoError(t, loaded.SaveWithoutKey())
			loaded, err = GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.Equal(t, "legacy-key", loaded.Key)
			require.Equal(t, "saved-without-key", loaded.Name)
			require.NoError(t, (&Channel{Id: channel.Id, Name: "updated", Group: "default"}).Update())
			loaded, err = GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.Equal(t, "updated", loaded.Name)
			require.Equal(t, "legacy-key", loaded.Key)
			createdBySave := Channel{Name: "save-created", Key: "second-key", Models: "model", Group: "1"}
			require.NoError(t, createdBySave.Save())
			batch := []Channel{{Name: "batch", Key: "third-key", Models: "model", Group: "default"}}
			require.NoError(t, BatchInsertChannels(batch))
			var batchCount int64
			require.NoError(t, DB.Model(&Channel{}).Where("name = ?", "batch").Count(&batchCount).Error)
			require.EqualValues(t, 1, batchCount)
			for _, operation := range []string{"insert", "save", "update", "batch"} {
				bound := Channel{Name: "cannot-bind", Key: "private-key", Models: "model", Group: "1", SupplierID: intPointer(1), SupplierProfileID: intPointer(1)}
				switch operation {
				case "insert":
					err = bound.Insert()
				case "save":
					err = bound.Save()
				case "update":
					bound.Id = channel.Id
					err = bound.UpdateWithSupplierPermission(true)
				case "batch":
					err = BatchInsertChannels([]Channel{bound})
				}
				require.ErrorContains(t, err, "before database migration", operation)
			}
		})
	}
}

func intPointer(value int) *int { return &value }

func TestSupplierGroupValidationUsesPersistedGroupsWhenNodeCacheIsStale(t *testing.T) {
	supplier, first, second := setupSupplierBindingTest(t)
	// Another node added group 7 and reused an old display name. This node's
	// cache still maps "coding" to group 2 and does not know group 7.
	require.NoError(t, DB.Create(&Option{Key: "PricingGroups", Value: `[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"renamed","ratio":2,"selectable":true},{"id":7,"name":"coding","ratio":2,"selectable":true}]`}).Error)
	require.NoError(t, DB.Create(&[]PricingGroupSupplierBinding{{GroupID: 2, SupplierID: supplier.ID, ProfileID: second.ID}, {GroupID: 7, SupplierID: supplier.ID, ProfileID: first.ID}}).Error)
	for _, reference := range []string{"7", "coding"} {
		t.Run(reference, func(t *testing.T) {
			unbound := Channel{Name: "invalid", Key: "key", Models: "model", Group: reference}
			require.Error(t, unbound.Insert(), "a new persisted bound group is never an arbitrary legacy group")
			valid := Channel{Name: "valid", Key: "key", Models: "model", Group: reference, SupplierID: &supplier.ID, SupplierProfileID: &first.ID}
			require.NoError(t, valid.Insert())
			require.Equal(t, "7", valid.Group, "canonicalization must use the database, not the stale name cache")
			loaded, err := GetChannelById(valid.Id, true)
			require.NoError(t, err)
			require.Equal(t, "7", loaded.Group)
			require.NoError(t, ValidateChannelPricingGroup(loaded, reference))
		})
	}
	missing := Channel{Name: "missing", Key: "key", Models: "model", Group: "unknown-legacy"}
	require.Error(t, missing.Insert(), "canonical persisted groups close the arbitrary-name fallback")
	_, bound, err := ResolveSupplierModelTariff("7", "model")
	require.True(t, bound)
	require.ErrorIs(t, err, ErrSupplierTariffUnavailable, "routing must wait for the group coefficient cache to refresh")
	require.NoError(t, DB.Where("key = ?", "PricingGroups").Delete(&Option{}).Error)
	require.NoError(t, missing.Insert(), "unmigrated installations retain arbitrary unbound legacy names")
}

func TestSupplierAtomicBindingChecksChannelsOutsideStaleNodeCache(t *testing.T) {
	supplier, first, _ := setupSupplierBindingTest(t)
	require.NoError(t, DB.Create(&Option{Key: "PricingGroups", Value: `[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":7,"name":"remote-group","ratio":2,"selectable":true}]`}).Error)
	channel := Channel{Name: "remote", Key: "key", Models: "model", Group: "7"}
	require.NoError(t, DB.Create(&channel).Error)
	patch := SupplierGroupBindingPatch{GroupID: 7, SupplierID: &supplier.ID, ProfileID: &first.ID}
	require.Error(t, ApplySupplierBindings([]SupplierGroupBindingPatch{patch}, nil), "an existing channel is affected even when its group is not in this node's cache")
	var binding PricingGroupSupplierBinding
	require.NoError(t, DB.Where("group_id = ?", 7).Limit(1).Find(&binding).Error)
	require.Zero(t, binding.SupplierID)
	name := "remote-group"
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{patch}, []SupplierChannelBindingPatch{{ChannelID: channel.Id, SupplierID: &supplier.ID, ProfileID: &first.ID, Group: &name}}))
	loaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, "7", loaded.Group)
	require.NoError(t, ValidateChannelPricingGroup(loaded, "7"))
}
