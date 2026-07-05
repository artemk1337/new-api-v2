package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorUsesEnglishGroupSaturationMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	respondTaskError(ctx, &dto.TaskError{
		Code:       "429",
		Message:    "upstream overloaded",
		StatusCode: http.StatusTooManyRequests,
	})

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.JSONEq(t, `{
		"code": "429",
		"message": "The upstream load for the current group is saturated. Please try again later or switch to another group.",
		"data": null
	}`, recorder.Body.String())
}

func TestErrorLogPricingGroupUsesSelectedPricingGroup(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "user-group")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "2")

	assert.Equal(t, "2", errorLogPricingGroup(ctx))
}

func TestErrorLogPricingGroupUsesAutoGroupWhenSelected(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "1")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "3")

	assert.Equal(t, "3", errorLogPricingGroup(ctx))
}
