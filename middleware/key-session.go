package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// KeySessionFingerprint identifies the credential without putting it in the
// signed (but readable) browser session cookie.
func KeySessionFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// KeySessionTokenAllowed checks current account and key state for every request.
// An exhausted key remains readable so its holder can inspect spending.
func KeySessionTokenAllowed(c *gin.Context, token *model.Token) bool {
	if token.Status != common.TokenStatusEnabled && token.Status != common.TokenStatusExhausted {
		return false
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
		return false
	}
	if ips := token.GetIpLimits(); len(ips) > 0 {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil || !common.IsIpInCIDRList(ip, ips) {
			return false
		}
	}
	user, err := model.GetUserById(token.UserId, false)
	return err == nil && user.Status == common.UserStatusEnabled
}

func KeySessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id, ok := session.Get("key_session_id").(int)
		fingerprint, fingerprintOK := session.Get("key_session_fingerprint").(string)
		if !ok || !fingerprintOK || id <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Unauthorized"})
			return
		}
		token, err := model.GetTokenById(id)
		if err != nil || subtle.ConstantTimeCompare([]byte(fingerprint), []byte(KeySessionFingerprint(token.Key))) != 1 || !KeySessionTokenAllowed(c, token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Unauthorized"})
			return
		}
		c.Set("key_session_token", token)
		c.Next()
	}
}
