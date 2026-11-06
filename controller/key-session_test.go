package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKeySessionLogsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previous
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	logs := make([]model.Log, 0, 1008)
	for i := 1; i <= 1005; i++ {
		logs = append(logs, model.Log{
			Id: i, TokenId: 7, ModelName: fmt.Sprintf("model-%d", i),
			Type: model.LogTypeConsume, CreatedAt: int64(i), Other: "{}",
		})
	}
	for i := 1006; i <= 1008; i++ {
		logs = append(logs, model.Log{Id: i, TokenId: 8, ModelName: "other-key", Other: "{}"})
	}
	require.NoError(t, db.CreateInBatches(logs, 100).Error)

	for _, tt := range []struct {
		query      string
		page       int
		count      int
		firstModel string
	}{
		{"", 1, 50, "model-1005"},
		{"?p=2&page_size=1000", 2, 50, "model-955"},
		{"?p=21", 21, 5, "model-5"},
		{"?p=22", 22, 0, ""},
		{"?p=-1", 1, 50, "model-1005"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/key-session/logs"+tt.query, nil)
			ctx.Set("key_session_token", &model.Token{Id: 7})
			KeySessionLogs(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Page     int              `json:"page"`
					PageSize int              `json:"page_size"`
					Total    int              `json:"total"`
					Items    []map[string]any `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			require.Equal(t, tt.page, response.Data.Page)
			require.Equal(t, 50, response.Data.PageSize)
			require.Equal(t, 1005, response.Data.Total)
			require.NotNil(t, response.Data.Items)
			require.Len(t, response.Data.Items, tt.count)
			if tt.count > 0 {
				require.Equal(t, tt.firstModel, response.Data.Items[0]["model_name"])
			}
			for _, item := range response.Data.Items {
				require.NotEqual(t, "other-key", item["model_name"])
				require.NotContains(t, item, "token_id")
				require.NotContains(t, item, "other")
			}
		})
	}
}
