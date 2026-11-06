package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestAdminTokenBlockSurvivesStaleCacheFill(t *testing.T) {
	truncateTables(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousClient, previousEnabled
		require.NoError(t, client.Close())
	})
	token := Token{UserId: 1, Key: "admin-block-cache", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, DB.Create(&token).Error)
	stale := token
	server.SetError("ERR Redis write unavailable")
	require.Error(t, SetTokenAdminStatus(&token, common.TokenStatusAdminDisabled))
	server.SetError("")
	stored, err := GetTokenByIds(token.Id, token.UserId)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusEnabled, stored.Status)
	require.NoError(t, SetTokenAdminStatus(&token, common.TokenStatusAdminDisabled))
	require.NoError(t, cacheSetToken(stale))
	_, err = ValidateUserToken(token.Key)
	require.ErrorIs(t, err, ErrTokenInvalid)

	blocked := token
	require.NoError(t, SetTokenAdminStatus(&token, common.TokenStatusEnabled))
	require.NoError(t, cacheSetToken(blocked))
	validated, err := ValidateUserToken(token.Key)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusEnabled, validated.Status)

	require.NoError(t, SetTokenAdminStatus(&token, common.TokenStatusAdminDisabled))
	require.NoError(t, DB.Delete(&token).Error)
	require.NoError(t, cacheDeleteToken(token.Key))
	require.NoError(t, cacheSetToken(stale))
	_, err = ValidateUserToken(token.Key)
	require.ErrorIs(t, err, ErrTokenInvalid)
}
