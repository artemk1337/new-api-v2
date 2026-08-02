package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyChatsOptionIsIgnored(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key IN ?", []string{removedChatsOptionKey, "Notice"}).Delete(&Option{}).Error)
	common.OptionMapRWMutex.Lock()
	optionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB.Where("key IN ?", []string{removedChatsOptionKey, "Notice"}).Delete(&Option{})
		common.OptionMapRWMutex.Lock()
		common.OptionMap = optionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, updateOptionMapFromDatabase(removedChatsOptionKey, `[{"Legacy":"https://example.test"}]`))
	common.OptionMapRWMutex.Lock()
	common.OptionMap[removedChatsOptionKey] = "stale"
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, updateOptionMapFromDatabase(removedChatsOptionKey, `[{"Legacy":"https://example.test"}]`))
	common.OptionMapRWMutex.RLock()
	_, loaded := common.OptionMap[removedChatsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, loaded)

	require.NoError(t, UpdateOption(removedChatsOptionKey, "[]"))
	var option Option
	err := DB.First(&option, "key = ?", removedChatsOptionKey).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	common.OptionMapRWMutex.Lock()
	common.OptionMap[removedChatsOptionKey] = "stale"
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, UpdateOptionsBulk(map[string]string{removedChatsOptionKey: "[]"}))
	err = DB.First(&option, "key = ?", removedChatsOptionKey).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	common.OptionMapRWMutex.RLock()
	_, loaded = common.OptionMap[removedChatsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, loaded)

	common.OptionMapRWMutex.Lock()
	common.OptionMap[removedChatsOptionKey] = "stale"
	common.OptionMapRWMutex.Unlock()
	err = UpdateOptionsBulk(map[string]string{
		removedChatsOptionKey:                 "[]",
		"ModelRequestRateLimitDurationActive": "true",
	})
	require.Error(t, err)
	common.OptionMapRWMutex.RLock()
	_, loaded = common.OptionMap[removedChatsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.True(t, loaded)

	require.NoError(t, UpdateOptionsBulk(map[string]string{removedChatsOptionKey: "[]", "Notice": "ok"}))
	common.OptionMapRWMutex.RLock()
	_, loaded = common.OptionMap[removedChatsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, loaded)
}
