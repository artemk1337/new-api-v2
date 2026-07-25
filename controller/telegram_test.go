package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func signedTelegramAuthorization(t *testing.T, token string, authDate time.Time) url.Values {
	t.Helper()

	params := url.Values{
		"auth_date":  {strconv.FormatInt(authDate.Unix(), 10)},
		"first_name": {"Telegram User"},
		"id":         {"123456"},
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params.Get(key))
	}

	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	_, err := mac.Write([]byte(strings.Join(parts, "\n")))
	require.NoError(t, err)
	params.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return params
}

func TestCheckTelegramAuthorization(t *testing.T) {
	token := "telegram-token"

	t.Run("accepts recent signed callback", func(t *testing.T) {
		params := signedTelegramAuthorization(t, token, time.Now())
		require.True(t, checkTelegramAuthorization(params, token))
	})

	t.Run("rejects expired callback", func(t *testing.T) {
		params := signedTelegramAuthorization(t, token, time.Now().Add(-telegramAuthorizationMaxAge-time.Second))
		require.False(t, checkTelegramAuthorization(params, token))
	})

	t.Run("rejects callback without telegram id", func(t *testing.T) {
		params := signedTelegramAuthorization(t, token, time.Now())
		params.Del("id")
		require.False(t, checkTelegramAuthorization(params, token))
	})
}

func TestTelegramBindRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:telegram-bind-session?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	originalDB := model.DB
	originalEnabled := common.TelegramOAuthEnabled
	originalToken := common.TelegramBotToken
	model.DB = db
	common.TelegramOAuthEnabled = true
	common.TelegramBotToken = "telegram-token"
	t.Cleanup(func() {
		model.DB = originalDB
		common.TelegramOAuthEnabled = originalEnabled
		common.TelegramBotToken = originalToken
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("telegram-bind-test"))))
	router.GET("/api/oauth/telegram/bind", TelegramBind)
	params := signedTelegramAuthorization(t, common.TelegramBotToken, time.Now())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/telegram/bind?"+params.Encode(), nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
