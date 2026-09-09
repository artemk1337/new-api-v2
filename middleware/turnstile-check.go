package middleware

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool   `json:"success"`
	Action  string `json:"action"`
}

var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
var turnstileHTTPClient = &http.Client{Timeout: 10 * time.Second}

func TurnstileCheck() gin.HandlerFunc {
	return turnstileCheck(true, "")
}

// TurnstileCheckOnce validates every request independently. Registration and
// email verification use it so one solved challenge authorizes one action.
func TurnstileCheckOnce(expectedAction string) gin.HandlerFunc {
	return turnstileCheck(false, expectedAction)
}

func turnstileCheck(reuseSession bool, expectedAction string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			if reuseSession {
				session := sessions.Default(c)
				if session.Get("turnstile") != nil {
					c.Next()
					return
				}
			}

			response := c.GetHeader("X-Turnstile-Token")
			if response == "" {
				response = c.Query("turnstile")
			}
			if response == "" {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}
			form := url.Values{
				"secret":   {common.TurnstileSecretKey},
				"response": {response},
				"remoteip": {c.ClientIP()},
			}
			request, err := http.NewRequestWithContext(
				c.Request.Context(),
				http.MethodPost,
				turnstileVerifyURL,
				strings.NewReader(form.Encode()),
			)
			if err == nil {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			var rawRes *http.Response
			if err == nil {
				rawRes, err = turnstileHTTPClient.Do(request)
			}
			if err != nil {
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
			defer rawRes.Body.Close()
			var res turnstileCheckResponse
			err = common.DecodeJson(io.LimitReader(rawRes.Body, 64<<10), &res)
			if err != nil {
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
			if !res.Success || (expectedAction != "" && res.Action != expectedAction) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
			if reuseSession {
				session := sessions.Default(c)
				session.Set("turnstile", true)
				err = session.Save()
				if err != nil {
					c.JSON(http.StatusOK, gin.H{
						"message": "无法保存会话信息，请重试",
						"success": false,
					})
					return
				}
			}
		}
		c.Next()
	}
}
