package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelDeletePathsClearPricingSyncOwnership(t *testing.T) {
	tests := []struct {
		name   string
		status int
		delete func(*Channel) error
	}{
		{
			name:   "single",
			status: common.ChannelStatusEnabled,
			delete: func(channel *Channel) error { return channel.Delete() },
		},
		{
			name:   "batch",
			status: common.ChannelStatusEnabled,
			delete: func(channel *Channel) error { return BatchDeleteChannels([]int{channel.Id}) },
		},
		{
			name:   "status",
			status: 42,
			delete: func(_ *Channel) error {
				_, err := DeleteChannelByStatus(42)
				return err
			},
		},
		{
			name:   "disabled",
			status: common.ChannelStatusManuallyDisabled,
			delete: func(_ *Channel) error {
				_, err := DeleteDisabledChannel()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.AutoMigrate(&Option{}))
			modelName := "delete-" + tt.name
			require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
				"ModelPrice": {Set: map[string]any{modelName: 1.0}},
			}))
			t.Cleanup(func() {
				require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
					"ModelPrice": {Delete: []string{modelName}},
				}))
			})

			channel := &Channel{Name: tt.name, Status: tt.status}
			require.NoError(t, DB.Create(channel).Error)
			require.NoError(t, DB.Create(&PricingSyncSource{
				ChannelID: channel.Id, Enabled: true, Endpoint: "/api/pricing",
			}).Error)
			require.NoError(t, DB.Create(&PricingSyncQuote{
				ChannelID: channel.Id, ModelName: modelName,
			}).Error)
			require.NoError(t, SavePricingSyncModelState(PricingSyncModelState{
				ModelName: modelName, Mode: PricingSyncModelModeChannel,
				ChannelID: channel.Id, Status: PricingSyncModelStatusReady,
			}))

			require.NoError(t, tt.delete(channel))

			var channelCount, sourceCount, quoteCount int64
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Count(&channelCount).Error)
			require.NoError(t, DB.Model(&PricingSyncSource{}).Where("channel_id = ?", channel.Id).Count(&sourceCount).Error)
			require.NoError(t, DB.Model(&PricingSyncQuote{}).Where("channel_id = ?", channel.Id).Count(&quoteCount).Error)
			assert.Zero(t, channelCount)
			assert.Zero(t, sourceCount)
			assert.Zero(t, quoteCount)

			state, err := GetPricingSyncModelState(modelName)
			require.NoError(t, err)
			assert.Equal(t, PricingSyncModelModeManual, state.Mode)
			assert.Equal(t, PricingSyncModelStatusUnavailable, state.Status)

			var option Option
			require.NoError(t, DB.First(&option, "key = ?", "ModelPrice").Error)
			assert.NotContains(t, option.Value, modelName)
		})
	}
}

func TestBatchDeleteChannelsRollsBackChannelWhenPricingCleanupFails(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	const modelName = "delete-rollback"
	channel := &Channel{Name: "rollback", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&PricingSyncSource{
		ChannelID: channel.Id, Enabled: true, Endpoint: "/api/pricing",
	}).Error)
	require.NoError(t, SavePricingSyncModelState(PricingSyncModelState{
		ModelName: modelName, Mode: PricingSyncModelModeChannel,
		ChannelID: channel.Id, Status: PricingSyncModelStatusReady,
	}))
	require.NoError(t, DB.Save(&Option{Key: "ModelPrice", Value: "not-json"}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Model(&Option{}).Where("key = ?", "ModelPrice").Update("value", "{}").Error)
	})

	err := BatchDeleteChannels([]int{channel.Id})

	require.Error(t, err)
	var channelCount, sourceCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Count(&channelCount).Error)
	require.NoError(t, DB.Model(&PricingSyncSource{}).Where("channel_id = ?", channel.Id).Count(&sourceCount).Error)
	assert.EqualValues(t, 1, channelCount)
	assert.EqualValues(t, 1, sourceCount)
}
