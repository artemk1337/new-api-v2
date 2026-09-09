package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationFrozenSelfAndBindingsRequireBrowserSession(t *testing.T) {
	for _, path := range []string{"/api/user/self", "/api/user/oauth/bindings"} {
		t.Run(path, func(t *testing.T) {
			resetVerificationAuthTestState(t)
			withVerificationAuthOptions(t)

			user := &model.User{
				Username:             "verification-auth-frozen",
				Password:             "password",
				Role:                 common.RoleCommonUser,
				Status:               model.UserStatusVerificationFrozen,
				VerificationFrozenAt: 100,
			}
			user.SetAccessToken("verification-auth-token")
			require.NoError(t, model.DB.Create(user).Error)

			browserResponse := performVerificationAuthRequest(t, path, user, false)
			require.Equal(t, http.StatusOK, browserResponse.Code)
			assert.JSONEq(t, `{"success":true}`, browserResponse.Body.String())

			tokenResponse := performVerificationAuthRequest(t, path, user, true)
			require.Equal(t, http.StatusOK, tokenResponse.Code)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(tokenResponse.Body.Bytes(), &response))
			assert.False(t, response.Success)
		})
	}
}

func resetVerificationAuthTestState(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM tokens").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)
}

func withVerificationAuthOptions(t *testing.T) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous := make(map[string]string)
	for _, key := range []string{
		setting.AccountVerificationEnabledOption,
		setting.AccountVerificationProvidersOption,
		setting.AccountVerificationFreezeDelayMinutesOption,
	} {
		if value, ok := common.OptionMap[key]; ok {
			previous[key] = value
		}
	}
	common.OptionMap[setting.AccountVerificationEnabledOption] = "true"
	common.OptionMap[setting.AccountVerificationProvidersOption] = "email"
	common.OptionMap[setting.AccountVerificationFreezeDelayMinutesOption] = "60"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for _, key := range []string{
			setting.AccountVerificationEnabledOption,
			setting.AccountVerificationProvidersOption,
			setting.AccountVerificationFreezeDelayMinutesOption,
		} {
			if value, ok := previous[key]; ok {
				common.OptionMap[key] = value
			} else {
				delete(common.OptionMap, key)
			}
		}
	})
}

func performVerificationAuthRequest(t *testing.T, path string, user *model.User, useAccessToken bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("verification-auth-test"))))
	router.GET("/login", func(c *gin.Context) {
		if useAccessToken {
			c.Status(http.StatusNoContent)
			return
		}
		session := sessions.Default(c)
		session.Set("username", user.Username)
		session.Set("role", user.Role)
		session.Set("id", user.Id)
		session.Set("status", user.Status)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET(path, UserAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	var cookies []*http.Cookie
	if !useAccessToken {
		loginRecorder := httptest.NewRecorder()
		loginRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
		router.ServeHTTP(loginRecorder, loginRequest)
		require.Equal(t, http.StatusNoContent, loginRecorder.Code)
		cookies = loginRecorder.Result().Cookies()
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	if useAccessToken {
		request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}
