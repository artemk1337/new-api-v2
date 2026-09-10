package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// IPBlacklist rejects requests before they reach any route-specific
// middleware (including registration and authentication). The source address
// comes exclusively from Gin's ClientIP resolution, which only considers
// forwarding headers when the server has explicitly configured trusted
// proxies.
func IPBlacklist() gin.HandlerFunc {
	return func(c *gin.Context) {
		if system_setting.IsIPBlacklisted(c.ClientIP()) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Request blocked",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
