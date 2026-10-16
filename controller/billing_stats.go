package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func billingQuotaPerUnit(c *gin.Context) (float64, bool) {
	quotaPerUnit := common.GetQuotaPerUnit()
	if !common.IsValidQuotaPerUnitValue(quotaPerUnit) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Invalid quota conversion configuration",
			"type":    "new_api_error",
		}})
		return 0, false
	}
	return quotaPerUnit, true
}

// GetAccountCredits returns the same wallet balance and lifetime usage as the dashboard.
func GetAccountCredits(c *gin.Context) {
	quotaPerUnit, ok := billingQuotaPerUnit(c)
	if !ok {
		return
	}
	user := c.MustGet("billing_user").(*model.User)
	balance := float64(user.Quota) / quotaPerUnit
	usage := float64(user.UsedQuota) / quotaPerUnit
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"total_credits": balance + usage,
		"total_usage":   usage,
		"balance":       balance,
		"currency":      "USD",
		"request_count": user.RequestCount,
	}})
}

// GetCurrentKeyCredits returns key-scoped limits independently of the wallet balance.
func GetCurrentKeyCredits(c *gin.Context) {
	quotaPerUnit, ok := billingQuotaPerUnit(c)
	if !ok {
		return
	}
	token := c.MustGet("billing_token").(*model.Token)
	var limit, remaining any
	if !token.UnlimitedQuota {
		remaining = float64(token.RemainQuota) / quotaPerUnit
		limit = (float64(token.RemainQuota) + float64(token.UsedQuota)) / quotaPerUnit
	}
	var expiresAt any
	if token.ExpiredTime > 0 {
		expiresAt = time.Unix(token.ExpiredTime, 0).UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"label":           token.Name,
		"limit":           limit,
		"limit_remaining": remaining,
		"usage":           float64(token.UsedQuota) / quotaPerUnit,
		"unlimited_quota": token.UnlimitedQuota,
		"expires_at":      expiresAt,
		"currency":        "USD",
	}})
}
