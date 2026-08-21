package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForLockedChannelPreservesSelectedKey(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "origin-key")
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, 2)
	mapping := `{"alias-model":"provider-model"}`
	channel := &model.Channel{Id: 42, Name: "locked", Key: "next-key", ModelMapping: &mapping}

	setupErr := SetupContextForLockedChannel(ctx, channel, "alias-model")
	require.Nil(t, setupErr, "%#v", setupErr)
	require.Equal(t, "origin-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, mapping, common.GetContextKeyString(ctx, constant.ContextKeyChannelModelMapping))
	require.Equal(t, 42, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 2, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}
