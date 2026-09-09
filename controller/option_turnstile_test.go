package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsTurnstileWithoutSecretKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalSiteKey := common.TurnstileSiteKey
	originalSecretKey := common.TurnstileSecretKey
	originalDB := model.DB
	model.DB = nil
	common.TurnstileSiteKey = "site-key"
	common.TurnstileSecretKey = ""
	t.Cleanup(func() {
		common.TurnstileSiteKey = originalSiteKey
		common.TurnstileSecretKey = originalSecretKey
		model.DB = originalDB
	})

	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "TurnstileCheckEnabled",
		Value: true,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	UpdateOption(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
}
