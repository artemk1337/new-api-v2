package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RegistrationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.AllowRegistrationAttempt(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many registration attempts. Please try again later.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
