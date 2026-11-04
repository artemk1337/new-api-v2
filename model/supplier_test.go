package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSupplierBindingTest(t *testing.T) (Supplier, SupplierPricingProfile, SupplierPricingProfile) {
	t.Helper()
	originalDB := DB
	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalMain, originalLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, DB.AutoMigrate(&Supplier{}, &SupplierPricingProfile{}, &PricingGroupSupplierBinding{}, &SupplierBindingState{}, &SupplierPricingState{}, &SupplierPricingRevision{}, &Channel{}, &PlatformTag{}, &Ability{}, &Midjourney{}, &Task{}, &Option{}, &PricingSyncSource{}))
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"coding","ratio":2,"selectable":true},{"id":3,"name":"other","ratio":2,"selectable":true}]`))
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalMain, originalLog)
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	supplier := Supplier{Code: "test-supplier", Name: "Test supplier", Adapter: "manual", Enabled: true}
	require.NoError(t, CreateSupplier(&supplier))
	first := SupplierPricingProfile{SupplierID: supplier.ID, Code: "first", Name: "First", Enabled: true, USDPerCredit: 0.006}
	second := SupplierPricingProfile{SupplierID: supplier.ID, Code: "second", Name: "Second", Enabled: true, USDPerCredit: 0.01}
	require.NoError(t, CreateSupplierPricingProfile(&first))
	require.NoError(t, CreateSupplierPricingProfile(&second))
	return supplier, first, second
}

func TestSupplierChannelRequiresExactProfileAndGroup(t *testing.T) {
	supplier, first, second := setupSupplierBindingTest(t)
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}, {GroupID: 3, SupplierID: &supplier.ID, ProfileID: &second.ID}}, nil))
	channel := Channel{Name: "bound", Key: "secret", Models: "model", Group: "2", SupplierID: &supplier.ID, SupplierProfileID: &first.ID}
	require.NoError(t, channel.Insert())
	require.NoError(t, ValidateChannelPricingGroup(&channel, "coding"))
	require.Error(t, ValidateChannelPricingGroup(&channel, "3"))
	for _, invalid := range []Channel{
		{Name: "wrong-profile", Key: "secret", Models: "model", Group: "3", SupplierID: &supplier.ID, SupplierProfileID: &first.ID},
		{Name: "default-channel", Key: "secret", Models: "model", Group: "2"},
		{Name: "default-group", Key: "secret", Models: "model", Group: "1", SupplierID: &supplier.ID, SupplierProfileID: &first.ID},
		{Name: "partial-pair", Key: "secret", Models: "model", Group: "2", SupplierID: &supplier.ID},
	} {
		require.Error(t, invalid.Insert(), invalid.Name)
	}
	supplier.Enabled = false
	require.NoError(t, UpdateSupplier(&supplier))
	require.Error(t, ValidateChannelPricingGroup(&channel, "2"))
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.SaveWithoutKey(), "operational changes must remain possible after disabling a supplier")
}

func TestSupplierBindingsApplyAtomicallyAndClearExplicitly(t *testing.T) {
	supplier, first, _ := setupSupplierBindingTest(t)
	channel := Channel{Name: "legacy", Key: "secret", Models: "model", Group: "2"}
	require.NoError(t, channel.Insert())
	groupPatch := SupplierGroupBindingPatch{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}
	require.Error(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, nil))
	binding, err := GetPricingGroupSupplierBinding("2")
	require.NoError(t, err)
	assert.Zero(t, binding.SupplierID, "failed migration must not leave a group binding")
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{groupPatch}, []SupplierChannelBindingPatch{{ChannelID: channel.Id, SupplierID: &supplier.ID, ProfileID: &first.ID}}))
	loaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NoError(t, ValidateChannelPricingGroup(loaded, "2"))
	loaded.SupplierID, loaded.SupplierProfileID = nil, nil
	require.Error(t, loaded.Update(), "clearing only the channel cannot escape a bound group")
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 2}}, []SupplierChannelBindingPatch{{ChannelID: channel.Id}}))
	loaded, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Nil(t, loaded.SupplierID)
	assert.Nil(t, loaded.SupplierProfileID)
}

func TestSupplierStaleChannelWriteCannotRemoveNewBinding(t *testing.T) {
	supplier, first, _ := setupSupplierBindingTest(t)
	channel := Channel{Name: "concurrent-edit", Key: "secret", Models: "model", Group: "2"}
	require.NoError(t, channel.Insert())
	stale := channel
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}}, []SupplierChannelBindingPatch{{ChannelID: channel.Id, SupplierID: &supplier.ID, ProfileID: &first.ID}}))
	stale.Group = "1"
	require.Error(t, stale.UpdateWithSupplierPermission(false), "permission must be checked against the locked live channel")
	require.Error(t, stale.SaveWithoutKey(), "an operational worker must not overwrite a newer binding")
	current, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "2", current.Group)
	assert.Equal(t, &supplier.ID, current.SupplierID)
	assert.Equal(t, &first.ID, current.SupplierProfileID)
	require.NoError(t, stale.UpdateWithSupplierPermission(true), "explicitly authorized changes remain available")
}

func TestSupplierProfileCASAndHistoryProtection(t *testing.T) {
	_, first, _ := setupSupplierBindingTest(t)
	require.NoError(t, DB.Model(&first).Updates(map[string]any{"last_attempt_at": 10, "last_success_at": 9, "last_error": "existing"}).Error)
	stale := first
	first.USDPerCredit = 0.012
	first.LastError = "forged"
	require.NoError(t, UpdateSupplierPricingProfile(&first))
	assert.Equal(t, int64(2), first.ConfigRevision)
	assert.Equal(t, "existing", first.LastError)
	require.Error(t, UpdateSupplierPricingProfile(&stale))
	first.USDPerCredit = 0
	require.Error(t, UpdateSupplierPricingProfile(&first))
	require.NoError(t, DB.Create(&SupplierPricingRevision{ProfileID: first.ID, Revision: 1, ConfigRevision: 1}).Error)
	require.Error(t, DeleteSupplierPricingProfile(first.ID), "immutable historical quotes must retain their profile identity")
}

func TestSupplierRebindWaitsForLegacyImageTasks(t *testing.T) {
	supplier, first, _ := setupSupplierBindingTest(t)
	channel := Channel{Name: "legacy-image", Key: "secret", Models: "model", Group: "2"}
	require.NoError(t, channel.Insert())
	task := Midjourney{ChannelId: channel.Id, Progress: "50%"}
	require.NoError(t, DB.Create(&task).Error)
	groups := []SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}}
	channels := []SupplierChannelBindingPatch{{ChannelID: channel.Id, SupplierID: &supplier.ID, ProfileID: &first.ID}}
	require.Error(t, ApplySupplierBindings(groups, channels))
	require.NoError(t, DB.Model(&task).Update("progress", "100%").Error)
	require.NoError(t, ApplySupplierBindings(groups, channels))
}

func TestSupplierRebindWaitsForEveryNonterminalAsyncTask(t *testing.T) {
	statuses := []TaskStatus{TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress, TaskStatusUnknown, "", "FUTURE_STATUS", TaskStatusSuccess, TaskStatusFailure}
	for _, transition := range []string{"bind", "unbind", "profile"} {
		for _, status := range statuses {
			t.Run(fmt.Sprintf("%s/%s", transition, status), func(t *testing.T) {
				supplier, first, second := setupSupplierBindingTest(t)
				channel := Channel{Name: "async-channel", Key: "secret", Models: "model", Group: "2"}
				require.NoError(t, channel.Insert())
				boundGroups := []SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}}
				boundChannels := []SupplierChannelBindingPatch{{ChannelID: channel.Id, SupplierID: &supplier.ID, ProfileID: &first.ID}}
				if transition != "bind" {
					require.NoError(t, ApplySupplierBindings(boundGroups, boundChannels))
				}
				require.NoError(t, DB.Create(&Task{ChannelId: channel.Id, Status: status, TaskID: "upstream-private-task"}).Error)
				groups, channels := boundGroups, boundChannels
				if transition == "unbind" {
					groups = []SupplierGroupBindingPatch{{GroupID: 2}}
					channels = []SupplierChannelBindingPatch{{ChannelID: channel.Id}}
				} else if transition == "profile" {
					groups[0].ProfileID = &second.ID
					channels[0].ProfileID = &second.ID
				}
				err := ApplySupplierBindings(groups, channels)
				if status == TaskStatusSuccess || status == TaskStatusFailure {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "asynchronous tasks are pending")
					loaded, loadErr := GetChannelById(channel.Id, true)
					require.NoError(t, loadErr)
					if transition == "bind" {
						assert.Nil(t, loaded.SupplierID)
					} else {
						assert.Equal(t, &first.ID, loaded.SupplierProfileID, "failed rebind must preserve task origin")
					}
				}
			})
		}
	}
}

func TestSupplierConnectionChangeWaitsForAsyncTasks(t *testing.T) {
	for _, change := range []string{"key", "base_url", "type"} {
		t.Run(change, func(t *testing.T) {
			supplier, first, _ := setupSupplierBindingTest(t)
			require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}}, nil))
			baseURL := "https://original.example"
			channel := Channel{Name: "async-connection", Type: 1, BaseURL: &baseURL, Key: "old-key", Models: "model", Group: "2", SupplierID: &supplier.ID, SupplierProfileID: &first.ID}
			require.NoError(t, channel.Insert())
			task := Task{ChannelId: channel.Id, Status: TaskStatusQueued}
			require.NoError(t, DB.Create(&task).Error)
			channel.Name = "renamed"
			require.NoError(t, channel.UpdateWithSupplierPermission(true), "ordinary metadata edits must remain available")
			switch change {
			case "key":
				channel.Key = "replacement-key"
			case "base_url":
				newURL := "https://replacement.example"
				channel.BaseURL = &newURL
			case "type":
				channel.Type = 2
			}
			require.ErrorContains(t, channel.UpdateWithSupplierPermission(true), "asynchronous tasks are pending")
			loaded, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, baseURL, loaded.GetBaseURL())
			assert.Equal(t, "old-key", loaded.Key)
			assert.Equal(t, 1, loaded.Type)
			require.NoError(t, DB.Model(&task).Update("status", TaskStatusSuccess).Error)
			require.NoError(t, channel.UpdateWithSupplierPermission(true))
		})
	}
}

func TestSupplierBoundGroupCannotBeRemovedByLegacyOptions(t *testing.T) {
	supplier, first, _ := setupSupplierBindingTest(t)
	require.NoError(t, ApplySupplierBindings([]SupplierGroupBindingPatch{{GroupID: 2, SupplierID: &supplier.ID, ProfileID: &first.ID}}, nil))
	require.Error(t, ValidateSupplierPricingGroupsJSON(`[{"id":1,"name":"default","ratio":1}]`))
	require.Error(t, ValidateSupplierPricingGroupsJSON(`{"default":1}`))
	require.NoError(t, ValidateSupplierPricingGroupsJSON(`[{"id":1,"name":"default","ratio":1},{"id":2,"name":"renamed","ratio":2}]`))
}

func TestSupplierMigrationPreviewPreservesStateAndSecrets(t *testing.T) {
	supplier, first, second := setupSupplierBindingTest(t)
	require.NoError(t, DeleteSupplierPricingProfile(second.ID))
	tag := supplier.Code
	channel := Channel{Name: "preview", Key: "secret-never-return", Models: "model", Group: "2", Tag: &tag}
	require.NoError(t, channel.Insert())
	preview, err := GetSupplierMigrationPreview()
	require.NoError(t, err)
	require.Len(t, preview.Channels, 1)
	assert.False(t, preview.AutomaticMigrationAllowed)
	assert.Equal(t, &first.ID, preview.Channels[0].ProposedProfileID)
	data, err := common.Marshal(preview)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-never-return")
	binding, err := GetPricingGroupSupplierBinding("2")
	require.NoError(t, err)
	assert.Zero(t, binding.SupplierID)
}
