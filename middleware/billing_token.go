package middleware

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BillingTokenAuth allows exhausted keys to inspect billing without granting relay access.
func BillingTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		key := c.GetHeader("Authorization")
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		key = strings.Split(strings.TrimPrefix(key, "sk-"), "-")[0]
		if key == "" {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenNotProvided))
			return
		}

		token, err := model.GetTokenByKey(key, true)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
			} else {
				common.SysError("BillingTokenAuth token lookup failed: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
			}
			return
		}
		if (token.Status != common.TokenStatusEnabled && token.Status != common.TokenStatusExhausted) ||
			(token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp()) {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
			return
		}
		if allowedIPs := token.GetIpLimits(); len(allowedIPs) > 0 {
			ip := net.ParseIP(c.ClientIP())
			if ip == nil || !common.IsIpInCIDRList(ip, allowedIPs) {
				abortWithOpenAiMessage(c, http.StatusForbidden, "Client IP is not allowed by this API key")
				return
			}
		}

		user, err := model.GetUserById(token.UserId, false)
		if err != nil {
			common.SysError("BillingTokenAuth user lookup failed: " + err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
			return
		}
		if user.Status != common.UserStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthUserBanned))
			return
		}
		c.Set("billing_user", user)
		c.Set("billing_token", token)
		c.Next()
	}
}
