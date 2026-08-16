package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func TestGenerateTextOtherInfoModelProvenance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &common.RelayInfo{
		RequestedModelName:        "opus-5",
		SentUpstreamModelName:     "anthropic/opus-5",
		ProviderReturnedModelName: "claude-opus-5-20250801",
		ChannelMeta:               &common.ChannelMeta{},
	}

	details := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	require.Equal(t, "opus-5", details["requested_model"])
	assert.Equal(t, "anthropic/opus-5", details["sent_upstream_model"])
	assert.Equal(t, "claude-opus-5-20250801", details["provider_returned_model"])
}

func TestGenerateTextOtherInfoOmitsUnknownProviderModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &common.RelayInfo{RequestedModelName: "opus-5", SentUpstreamModelName: "opus-5", ChannelMeta: &common.ChannelMeta{}}

	details := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	_, ok := details["provider_returned_model"]
	assert.False(t, ok)
}
