package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMain(m *testing.M) {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	common.SQLitePath = filepath.Join(os.TempDir(), "new-api-middleware-auth-test.db")
	_ = os.Remove(common.SQLitePath)

	if err := model.InitDB(); err != nil {
		panic("failed to init test db: " + err.Error())
	}

	code := m.Run()
	_ = os.Remove(common.SQLitePath)
	os.Exit(code)
}

func TestTokenAuthReturnsEnglishForUnavailableTokenGroup(t *testing.T) {
	resetTokenAuthTestState(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"claude":1}`))
	createTokenAuthUserAndToken(t, "default", "claude")

	body := performTokenAuthRequest(t, http.StatusForbidden)

	assert.Equal(t, "No permission to access claude group (request id: test-request-id)", body.Get("error.message").String())
	assert.Equal(t, "new_api_error", body.Get("error.type").String())
}

func TestTokenAuthReturnsEnglishForDeprecatedTokenGroup(t *testing.T) {
	resetTokenAuthTestState(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","claude":"Claude"}`))
	createTokenAuthUserAndToken(t, "default", "claude")

	body := performTokenAuthRequest(t, http.StatusForbidden)

	assert.Equal(t, "Group claude has been deprecated (request id: test-request-id)", body.Get("error.message").String())
	assert.Equal(t, "new_api_error", body.Get("error.type").String())
}

func TestTokenAuthAllowsAutoTokenGroup(t *testing.T) {
	resetTokenAuthTestState(t)
	createTokenAuthUserAndToken(t, "default", "auto")

	body := performTokenAuthRequest(t, http.StatusOK)

	assert.Equal(t, "auto", body.Get("using_group").String())
	assert.Equal(t, "auto", body.Get("token_group").String())
}

func TestTokenAuthDoesNotTreatNumericUserGroupAsPricingID(t *testing.T) {
	resetTokenAuthTestState(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))
	createTokenAuthUserAndToken(t, "2", "")

	body := performTokenAuthRequest(t, http.StatusOK)

	assert.Equal(t, "1", body.Get("using_group").String())
	assert.Empty(t, body.Get("token_group").String())
}

func resetTokenAuthTestState(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM tokens").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
}

func createTokenAuthUserAndToken(t *testing.T, userGroup, tokenGroup string) {
	t.Helper()
	user := model.User{
		Id:       1001,
		Username: "token-auth-user",
		Password: "not-used-in-test",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    userGroup,
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)

	token := model.Token{
		UserId:         user.Id,
		Key:            "test",
		Status:         common.TokenStatusEnabled,
		Name:           "test token",
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          tokenGroup,
	}
	require.NoError(t, model.DB.Create(&token).Error)
}

func performTokenAuthRequest(t *testing.T, expectedStatus int) gjson.Result {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "test-request-id")
		c.Next()
	})
	router.Use(TokenAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"token_group": common.GetContextKeyString(c, constant.ContextKeyTokenGroup),
			"using_group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer sk-test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, expectedStatus, resp.Code)
	return gjson.Parse(resp.Body.String())
}
