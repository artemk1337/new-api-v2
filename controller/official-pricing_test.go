package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOfficialPriceAPIRequiresCompleteRecordsAndReturnsStoredQuoteDate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "official-price-api.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.OfficialModelPrice{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	router := gin.New()
	router.GET("/official-prices", GetOfficialPrices)
	router.PUT("/official-prices", UpdateOfficialPrices)
	router.DELETE("/official-prices", DeleteOfficialPrice)

	request := func(payload map[string]any) (bool, []model.OfficialModelPrice) {
		t.Helper()
		body, err := common.Marshal(payload)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/official-prices", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		var response struct {
			Success bool                       `json:"success"`
			Data    []model.OfficialModelPrice `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success, response.Data
	}
	quoteDate := time.Now().Add(-24 * time.Hour).Unix()
	record := map[string]any{
		"model_name": "exact-model", "input_usd": 0, "output_usd": 12,
		"source": "https://platform.openai.com/docs/pricing", "updated_at": quoteDate, "price_unit": "million_tokens",
	}
	success, prices := request(map[string]any{"prices": []any{record}})
	require.True(t, success)
	require.Len(t, prices, 1)
	require.Zero(t, *prices[0].InputUSD)
	require.Equal(t, quoteDate, prices[0].UpdatedAt)

	delete(record, "output_usd")
	record["input_usd"] = 100
	success, _ = request(map[string]any{"prices": []any{record}})
	require.False(t, success)
	stored, err := model.ListOfficialModelPrices()
	require.NoError(t, err)
	require.Zero(t, *stored[0].InputUSD)
	require.Equal(t, 12.0, *stored[0].OutputUSD)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/official-prices", bytes.NewBufferString(`{"model_name":"exact-model"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	var response struct {
		Success bool                       `json:"success"`
		Data    []model.OfficialModelPrice `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Empty(t, response.Data)
}
