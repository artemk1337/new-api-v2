package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributeModelAliasUsesCanonicalAndPreservesAccessChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Channel{}, &model.Option{}))
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Create(&model.Model{ModelName: "canonical-model", Aliases: model.ModelAliases{"cursor-model"}, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 42, Name: "locked", Key: "key", Status: common.ChannelStatusEnabled}).Error)

	for _, tc := range []struct {
		name, allowed, mapping, target string
		status                         int
	}{
		{"alias", "cursor-model", "", "canonical-model", http.StatusNoContent},
		{"canonical permission cannot bypass alias restriction", "canonical-model", "", "", http.StatusForbidden},
		{"key override", "cursor-model", `{"cursor-model":"key-target"}`, "key-target", http.StatusNoContent},
		{"mapping cycle", "cursor-model", `{"canonical-model":"cursor-model"}`, "", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "42")
				common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
				common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{tc.allowed: true})
				common.SetContextKey(c, constant.ContextKeyTokenModelMapping, tc.mapping)
			}, Distribute(), func(c *gin.Context) {
				require.Equal(t, tc.target, c.GetString("original_model"))
				require.Equal(t, "cursor-model", common.GetContextKeyString(c, constant.ContextKeyRequestedModel))
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cursor-model"}`))
			request.Header.Set("Content-Type", gin.MIMEJSON)
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
		})
	}
}
