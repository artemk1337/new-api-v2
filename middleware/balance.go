package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Reject unfunded API requests before channel selection and billing log creation.
func rejectEmptyBalance(c *gin.Context) bool {
	quota, ok := common.GetContextKeyType[int](c, constant.ContextKeyUserQuota)
	if !ok || quota > 0 {
		return false
	}
	setting, _ := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if common.NormalizeBillingPreference(setting.BillingPreference) != "wallet_only" {
		hasSubscription, err := model.HasActiveUserSubscription(c.GetInt("id"))
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to check subscription", types.ErrorCodeQueryDataError)
			return true
		}
		if hasSubscription {
			return false
		}
	}
	// Expected rejections must not generate a usage row or an application error log.
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId("Insufficient balance. Please add funds to continue.", c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    types.ErrorCodeInsufficientUserQuota,
		},
	})
	return true
}
