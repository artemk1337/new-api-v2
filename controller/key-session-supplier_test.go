package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func keySessionSupplierRouter() *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("key-session-supplier-test-secret"))))
	router.POST("/api/key-login", KeySessionLogin)
	limited := router.Group("/api/key-session", middleware.KeySessionAuth())
	limited.GET("", KeySessionInfo)
	limited.GET("/logs", KeySessionLogs)
	router.GET("/api/suppliers", middleware.RootAuth(), GetSuppliers)
	router.GET("/api/suppliers/choices", middleware.AdminAuth(), GetSupplierChoices)
	router.PUT("/api/suppliers/group-bindings", middleware.RootAuth(), UpdateSupplierGroupBindings)
	router.POST("/api/suppliers/profiles/:id/tariffs", middleware.RootAuth(), PublishSupplierPricingTariffs)
	return router
}

func requestKeySessionSupplier(router *gin.Engine, method, path string, body []byte, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func loginKeySessionSupplier(t *testing.T, router *gin.Engine, token *model.Token, previous *http.Cookie) *http.Cookie {
	t.Helper()
	body, err := common.Marshal(map[string]string{"key": "sk-" + token.Key})
	require.NoError(t, err)
	response := requestKeySessionSupplier(router, http.MethodPost, "/api/key-login", body, previous)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.JSONEq(t, `{"success":true,"message":""}`, response.Body.String())
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func TestKeySessionSupplierDetailsStayPrivate(t *testing.T) {
	initModelListColumnNames(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	token := seedToken(t, db, 1, "Public API key", "private-api-key")
	token.ModelMapping = `{"public-model":"private-upstream-model"}`
	require.NoError(t, db.Save(token).Error)
	sibling := seedToken(t, db, 1, "Another key", "another-private-key")
	privateOther := `{"supplier_pricing":true,"requested_model":"public-model","supplier_id":321,"profile_id":654,"margin":2,"usd_per_credit":0.006,"supplier_tariff":{"input_usd":0.012},"admin_info":{"key":"private-upstream-key"},"raw_response":"private supplier error"}`
	require.NoError(t, db.Create(&[]model.Log{
		{TokenId: token.Id, CreatedAt: 123, Type: model.LogTypeError, ModelName: "private-upstream-model", ChannelId: 123, UpstreamRequestId: "private-upstream-request", Content: "private supplier error", Other: privateOther, PromptTokens: 11, CompletionTokens: 7, Quota: 40, UseTime: 3},
		{TokenId: sibling.Id, Type: model.LogTypeConsume, ModelName: "sibling-model", Quota: 999},
	}).Error)
	router := keySessionSupplierRouter()
	limitedCookie := loginKeySessionSupplier(t, router, token, nil)

	info := requestKeySessionSupplier(router, http.MethodGet, "/api/key-session", nil, limitedCookie)
	require.Equal(t, http.StatusOK, info.Code)
	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(info.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, map[string]any{
		"name": token.Name, "used_quota": float64(token.UsedQuota), "remain_quota": float64(token.RemainQuota),
		"unlimited_quota": token.UnlimitedQuota, "expired_time": float64(token.ExpiredTime),
	}, payload.Data)

	logs := requestKeySessionSupplier(router, http.MethodGet, "/api/key-session/logs?token_id="+strconv.Itoa(sibling.Id), nil, limitedCookie)
	require.Equal(t, http.StatusOK, logs.Code)
	var logPayload struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(logs.Body.Bytes(), &logPayload))
	require.True(t, logPayload.Success)
	require.Len(t, logPayload.Data, 1)
	require.Equal(t, map[string]any{
		"id": float64(1), "created_at": float64(123), "type": float64(model.LogTypeError), "model_name": "public-model", "quota": float64(40),
		"prompt_tokens": float64(11), "completion_tokens": float64(7), "use_time": float64(3), "is_stream": false,
	}, logPayload.Data[0])
	for _, response := range []*httptest.ResponseRecorder{info, logs} {
		for _, secret := range []string{"supplier", "profile", "margin", "tariff", "usd_per_credit", "private-", "model_mapping", "channel", "other", "raw_response"} {
			require.NotContains(t, response.Body.String(), secret)
		}
	}
	var stored model.Log
	require.NoError(t, db.Where("token_id = ?", token.Id).First(&stored).Error)
	require.Equal(t, privateOther, stored.Other)
}

func TestKeySessionSupplierAPIsRejectKeyOnlyIdentity(t *testing.T) {
	initModelListColumnNames(t)
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "Root-owned API key", "root-owned-api-key")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", token.UserId).Update("role", common.RoleRootUser).Error)
	router := keySessionSupplierRouter()
	router.POST("/test/root-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", token.UserId)
		session.Set("username", "root")
		session.Set("role", common.RoleRootUser)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
		c.Status(http.StatusOK)
	})
	router.GET("/test/root", middleware.RootAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	rootLogin := requestKeySessionSupplier(router, http.MethodPost, "/test/root-session", nil, nil)
	rootCookie := rootLogin.Result().Cookies()[0]
	rootRequest := httptest.NewRequest(http.MethodGet, "/test/root", nil)
	rootRequest.AddCookie(rootCookie)
	rootRequest.Header.Set("New-Api-User", strconv.Itoa(token.UserId))
	rootResponse := httptest.NewRecorder()
	router.ServeHTTP(rootResponse, rootRequest)
	require.Equal(t, http.StatusNoContent, rootResponse.Code)

	limitedCookie := loginKeySessionSupplier(t, router, token, rootCookie)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/test/root"},
		{http.MethodGet, "/api/suppliers"},
		{http.MethodGet, "/api/suppliers/choices"},
		{http.MethodPut, "/api/suppliers/group-bindings"},
		{http.MethodPost, "/api/suppliers/profiles/1/tariffs"},
	} {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			req.AddCookie(limitedCookie)
			req.Header.Set("New-Api-User", strconv.Itoa(token.UserId))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
			require.NotContains(t, response.Body.String(), "supplier_id")
		})
	}
	info := requestKeySessionSupplier(router, http.MethodGet, "/api/key-session", nil, limitedCookie)
	require.Equal(t, http.StatusOK, info.Code)
}

func TestKeySessionSupplierRevalidatesCredential(t *testing.T) {
	for _, test := range []struct {
		name        string
		tokenUpdate map[string]any
		userUpdate  map[string]any
		deleteToken bool
		status      int
	}{
		{name: "rotated", tokenUpdate: map[string]any{"key": "rotated-api-key"}, status: http.StatusUnauthorized},
		{name: "disabled", tokenUpdate: map[string]any{"status": common.TokenStatusDisabled}, status: http.StatusUnauthorized},
		{name: "expired", tokenUpdate: map[string]any{"expired_time": common.GetTimestamp() - 1}, status: http.StatusUnauthorized},
		{name: "deleted", deleteToken: true, status: http.StatusUnauthorized},
		{name: "owner-disabled", userUpdate: map[string]any{"status": common.UserStatusDisabled}, status: http.StatusUnauthorized},
		{name: "ip-restricted", tokenUpdate: map[string]any{"allow_ips": "198.51.100.1"}, status: http.StatusUnauthorized},
		{name: "exhausted-readable", tokenUpdate: map[string]any{"status": common.TokenStatusExhausted, "remain_quota": 0}, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			initModelListColumnNames(t)
			db := setupTokenControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			token := seedToken(t, db, 1, "Public API key", "test-api-key")
			router := keySessionSupplierRouter()
			limitedCookie := loginKeySessionSupplier(t, router, token, nil)
			if test.tokenUpdate != nil {
				require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(test.tokenUpdate).Error)
			}
			if test.userUpdate != nil {
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", token.UserId).Updates(test.userUpdate).Error)
			}
			if test.deleteToken {
				require.NoError(t, db.Delete(token).Error)
			}
			for _, path := range []string{"/api/key-session", "/api/key-session/logs"} {
				response := requestKeySessionSupplier(router, http.MethodGet, path, nil, limitedCookie)
				require.Equal(t, test.status, response.Code, response.Body.String())
			}
		})
	}
}
