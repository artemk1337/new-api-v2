package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegisterReleasesSuccessfulAccountSlotWhenDatabaseTransactionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:register-rate-limit?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		operation_setting.RegistrationRateLimitEnabled:       "true",
		operation_setting.RegistrationRateLimitAttempts:      "10",
		operation_setting.RegistrationRateLimitSuccesses:     "1",
		operation_setting.RegistrationRateLimitWindowMinutes: "60",
	}
	common.OptionMapRWMutex.Unlock()
	previousRedisEnabled := common.RedisEnabled
	previousRegisterEnabled := common.RegisterEnabled
	previousPasswordRegisterEnabled := common.PasswordRegisterEnabled
	previousEmailVerificationEnabled := common.EmailVerificationEnabled
	previousQuotaForNewUser := common.QuotaForNewUser
	previousGenerateDefaultToken := constant.GenerateDefaultToken
	common.RedisEnabled = false
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		common.RedisEnabled = previousRedisEnabled
		common.RegisterEnabled = previousRegisterEnabled
		common.PasswordRegisterEnabled = previousPasswordRegisterEnabled
		common.EmailVerificationEnabled = previousEmailVerificationEnabled
		common.QuotaForNewUser = previousQuotaForNewUser
		constant.GenerateDefaultToken = previousGenerateDefaultToken
	})

	register := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewBufferString(body))
		ctx.Request.RemoteAddr = "203.0.113.88:12345"
		ctx.Request.Header.Set("Content-Type", "application/json")
		Register(ctx)
		return recorder
	}

	failed := register(`{"username":"failed-user","password":"password123","aff_code":"missing"}`)
	require.Equal(t, http.StatusOK, failed.Code)
	assert.Contains(t, failed.Body.String(), "Referral code is invalid")

	succeeded := register(`{"username":"valid-user","password":"password123"}`)
	require.Equal(t, http.StatusOK, succeeded.Code, succeeded.Body.String())
	assert.Contains(t, succeeded.Body.String(), `"success":true`)

	blocked := register(`{"username":"blocked-user","password":"password123"}`)
	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
}
