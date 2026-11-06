package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func adminTokenRequest(t *testing.T, role, userId, tokenId int, method, body string) gjson.Result {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 999)
		c.Set("role", role)
	})
	router.GET("/users/:id/tokens", GetUserTokensByAdmin)
	router.PUT("/users/:id/tokens/:token_id/status", SetUserTokenStatusByAdmin)
	path := fmt.Sprintf("/users/%d/tokens", userId)
	if method == http.MethodPut {
		path += fmt.Sprintf("/%d/status", tokenId)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return gjson.Parse(response.Body.String())
}

func TestAdminTokenManagement(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	user := model.User{Id: 10, Username: "managed", Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "secret-token", Name: "key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)

	body := adminTokenRequest(t, common.RoleAdminUser, user.Id, 0, http.MethodGet, "")
	require.True(t, body.Get("success").Bool())
	require.EqualValues(t, 1, body.Get("data.total").Int())
	require.Equal(t, "key", body.Get("data.items.0.name").String())
	require.False(t, body.Get("data.items.0.key").Exists())

	body = adminTokenRequest(t, common.RoleAdminUser, user.Id, token.Id, http.MethodPut, `{"status":2}`)
	require.True(t, body.Get("success").Bool())
	require.EqualValues(t, common.TokenStatusAdminDisabled, body.Get("data.status").Int())
	stored, err := model.GetTokenByIds(token.Id, user.Id)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusAdminDisabled, stored.Status)

	// A previously read owner update must not undo the administrative block.
	token.Name = "stale update"
	require.ErrorIs(t, token.Update(), model.ErrTokenAdminDisabled)
	require.ErrorIs(t, token.SelectUpdate(), model.ErrTokenAdminDisabled)

	ownerRouter := gin.New()
	ownerRouter.Use(func(c *gin.Context) { c.Set("id", user.Id) })
	ownerRouter.PUT("/token", UpdateToken)
	req := httptest.NewRequest(http.MethodPut, "/token?status_only=true", strings.NewReader(fmt.Sprintf(`{"id":%d,"status":1}`, token.Id)))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	ownerRouter.ServeHTTP(resp, req)
	require.False(t, gjson.Get(resp.Body.String(), "success").Bool())

	body = adminTokenRequest(t, common.RoleAdminUser, user.Id, token.Id, http.MethodPut, `{"status":1}`)
	require.True(t, body.Get("success").Bool())
	stored, err = model.GetTokenByIds(token.Id, user.Id)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusEnabled, stored.Status)
}

func TestAdminTokenManagementPermissionAndOwnership(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	users := []model.User{
		{Id: 10, Username: "first", AffCode: "first", Role: common.RoleCommonUser},
		{Id: 11, Username: "second", AffCode: "second", Role: common.RoleCommonUser},
		{Id: 12, Username: "peer", AffCode: "peer", Role: common.RoleAdminUser},
	}
	require.NoError(t, db.Create(&users).Error)
	token := model.Token{UserId: 11, Key: "other-secret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	for _, test := range []struct {
		role, userId, tokenId int
		method, body          string
	}{
		{common.RoleCommonUser, 10, 0, http.MethodGet, ""},
		{common.RoleAdminUser, 12, 0, http.MethodGet, ""},
		{common.RoleAdminUser, 10, token.Id, http.MethodPut, `{"status":2}`},
		{common.RoleAdminUser, 11, token.Id, http.MethodPut, `{"status":5}`},
	} {
		body := adminTokenRequest(t, test.role, test.userId, test.tokenId, test.method, test.body)
		require.False(t, body.Get("success").Bool())
	}
	stored, err := model.GetTokenByIds(token.Id, token.UserId)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusEnabled, stored.Status)
}

func TestAdminEnableTokenPreservesValidity(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	user := model.User{Id: 10, Username: "managed", Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	for _, test := range []struct {
		name   string
		token  model.Token
		status int
	}{
		{"expired", model.Token{ExpiredTime: 1, UnlimitedQuota: true}, common.TokenStatusExpired},
		{"exhausted", model.Token{ExpiredTime: -1}, common.TokenStatusExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := test.token
			token.Key = test.name
			token.UserId = user.Id
			token.Status = common.TokenStatusAdminDisabled
			require.NoError(t, db.Create(&token).Error)
			body := adminTokenRequest(t, common.RoleAdminUser, user.Id, token.Id, http.MethodPut, `{"status":1}`)
			require.True(t, body.Get("success").Bool())
			require.EqualValues(t, test.status, body.Get("data.status").Int())
		})
	}
}
