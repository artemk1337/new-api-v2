package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEmptyBalanceRejectsBeforeRelay(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/videos/task/remix"} {
		for _, quota := range []int{0, -5} {
			t.Run(fmt.Sprintf("%s/quota=%d", path, quota), func(t *testing.T) {
				resetTokenAuthTestState(t)
				createTokenAuthUserAndToken(t, "default", "auto")
				require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1001).Update("quota", quota).Error)
				require.NoError(t, model.DB.Where("user_id = ?", 1001).Delete(&model.UserSubscription{}).Error)
				var logsBefore, logsAfter int64
				require.NoError(t, model.DB.Model(&model.Log{}).Count(&logsBefore).Error)
				router := gin.New()
				router.POST(path, TokenAuth(), Distribute(), func(c *gin.Context) {
					t.Fatal("unfunded request reached relay")
				})
				for range 3 {
					request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"test-model"}`))
					request.Header.Set("Authorization", "Bearer sk-test")
					request.Header.Set("Content-Type", gin.MIMEJSON)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
					require.Equal(t, "insufficient_user_quota", gjson.Get(response.Body.String(), "error.code").String())
				}
				require.NoError(t, model.DB.Model(&model.Log{}).Count(&logsAfter).Error)
				require.Equal(t, logsBefore, logsAfter)
			})
		}
	}
}

func TestEmptyBalanceFundingSources(t *testing.T) {
	for _, test := range []struct {
		name         string
		quota        int
		preference   string
		subStatus    string
		subEnd       int64
		wantRejected bool
	}{
		{name: "empty", wantRejected: true},
		{name: "positive", quota: 1},
		{name: "active subscription", subStatus: "active", subEnd: time.Now().Unix() + 3600},
		{name: "wallet only", preference: "wallet_only", subStatus: "active", subEnd: time.Now().Unix() + 3600, wantRejected: true},
		{name: "wallet first fallback", preference: "wallet_first", subStatus: "active", subEnd: time.Now().Unix() + 3600},
		{name: "subscription only", preference: "subscription_only", subStatus: "active", subEnd: time.Now().Unix() + 3600},
		{name: "expired", subStatus: "active", subEnd: time.Now().Unix() - 1, wantRejected: true},
		{name: "cancelled", subStatus: "cancelled", subEnd: time.Now().Unix() + 3600, wantRejected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetTokenAuthTestState(t)
			createTokenAuthUserAndToken(t, "default", "auto")
			require.NoError(t, model.DB.Where("user_id = ?", 1001).Delete(&model.UserSubscription{}).Error)
			if test.subStatus != "" {
				require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 1001, Status: test.subStatus, EndTime: test.subEnd}).Error)
			}
			setting, err := common.Marshal(map[string]string{"billing_preference": test.preference})
			require.NoError(t, err)
			user := model.UserBase{Id: 1001, Quota: test.quota, Setting: string(setting)}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("id", user.Id)
			user.WriteContext(ctx)
			require.Equal(t, test.wantRejected, rejectEmptyBalance(ctx))
			// A top-up must unblock the next request without a negative-balance cache.
			common.SetContextKey(ctx, constant.ContextKeyUserQuota, 1)
			require.False(t, rejectEmptyBalance(ctx))
		})
	}
}

func TestEmptyBalanceAllowsModelsAndTaskFetch(t *testing.T) {
	resetTokenAuthTestState(t)
	createTokenAuthUserAndToken(t, "default", "auto")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1001).Update("quota", 0).Error)
	router := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router.GET("/v1/models", TokenAuth(), ok)
	router.GET("/suno/fetch/:id", TokenAuth(), Distribute(), ok)
	for _, path := range []string{"/v1/models", "/suno/fetch/task"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer sk-test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	}
}
