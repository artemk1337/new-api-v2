package controller

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tokenAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type tokenPageResponse struct {
	Items []tokenResponseItem `json:"items"`
}

type tokenResponseItem struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	Key                 string   `json:"key"`
	Status              int      `json:"status"`
	Group               string   `json:"group"`
	AutoGroupCandidates []string `json:"auto_group_candidates"`
	ModelMapping        string   `json:"model_mapping"`
}

type tokenKeyResponse struct {
	Key string `json:"key"`
}

type sqliteColumnInfo struct {
	Name string `gorm:"column:name"`
	Type string `gorm:"column:type"`
}

type legacyToken struct {
	Id                 int    `gorm:"primaryKey"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"column:key;type:char(48);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"column:group;default:''"`
	CrossGroupRetry    bool
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacyToken) TableName() string {
	return "tokens"
}

func openTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func migrateTokenControllerTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(&model.Token{}); err != nil {
		t.Fatalf("failed to migrate token table: %v", err)
	}
}

func setupTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	migrateTokenControllerTestDB(t, db)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	return db
}

func openTokenControllerExternalDB(t *testing.T, dialect string, dsn string) (*gorm.DB, *bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false

	var (
		db     *gorm.DB
		dbType common.DatabaseType
		err    error
	)
	switch dialect {
	case "mysql":
		dbType = common.DatabaseTypeMySQL
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		dbType = common.DatabaseTypePostgreSQL
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	common.SetDatabaseTypes(dbType, dbType)
	if err != nil {
		t.Fatalf("failed to open %s db: %v", dialect, err)
	}

	model.DB = db
	model.LOG_DB = db

	if db.Migrator().HasTable("tokens") {
		t.Skipf("refusing to run %s migration compatibility test against external database because tokens table already exists", dialect)
	}

	managedTokensTable := new(bool)

	t.Cleanup(func() {
		if *managedTokensTable && db.Migrator().HasTable("tokens") {
			_ = db.Migrator().DropTable("tokens")
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db, managedTokensTable
}

func seedToken(t *testing.T, db *gorm.DB, userID int, name string, rawKey string) *model.Token {
	t.Helper()

	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Count(&userCount).Error)
	if userCount == 0 {
		require.NoError(t, db.Create(&model.User{
			Id:       userID,
			Username: fmt.Sprintf("token-user-%d", userID),
			Password: "not-used-in-test",
			Group:    "default",
			Status:   common.UserStatusEnabled,
			AffCode:  fmt.Sprintf("token-%d", userID),
		}).Error)
	}
	token := &model.Token{
		UserId:         userID,
		Name:           name,
		Key:            rawKey,
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	return token
}

func newAuthenticatedContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) tokenAPIResponse {
	t.Helper()

	var response tokenAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	return response
}

func getSQLiteColumnType(t *testing.T, db *gorm.DB, tableName string, columnName string) string {
	t.Helper()

	var columns []sqliteColumnInfo
	if err := db.Raw("PRAGMA table_info(" + tableName + ")").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect %s schema: %v", tableName, err)
	}

	for _, column := range columns {
		if column.Name == columnName {
			return strings.ToLower(column.Type)
		}
	}

	t.Fatalf("column %s not found in %s schema", columnName, tableName)
	return ""
}

func getTokenKeyColumnType(t *testing.T, db *gorm.DB, dialect string) string {
	t.Helper()

	switch dialect {
	case "sqlite":
		return getSQLiteColumnType(t, db, "tokens", "key")
	case "mysql":
		var columnType string
		if err := db.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Scan(&columnType).Error; err != nil {
			t.Fatalf("failed to inspect mysql token key column: %v", err)
		}
		return strings.ToLower(columnType)
	case "postgres":
		var dataType string
		var maxLength sql.NullInt64
		if err := db.Raw(`SELECT data_type, character_maximum_length
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Row().Scan(&dataType, &maxLength); err != nil {
			t.Fatalf("failed to inspect postgres token key column: %v", err)
		}
		switch strings.ToLower(dataType) {
		case "character varying":
			return fmt.Sprintf("varchar(%d)", maxLength.Int64)
		case "character":
			return fmt.Sprintf("char(%d)", maxLength.Int64)
		default:
			if maxLength.Valid {
				return fmt.Sprintf("%s(%d)", strings.ToLower(dataType), maxLength.Int64)
			}
			return strings.ToLower(dataType)
		}
	default:
		t.Fatalf("unsupported dialect %q", dialect)
		return ""
	}
}

func runTokenMigrationCompatibilityTest(t *testing.T, db *gorm.DB, dialect string, managedTokensTable *bool) {
	t.Helper()

	legacyKey := strings.Repeat("a", 48)
	longKey := strings.Repeat("b", 64)

	if err := db.AutoMigrate(&legacyToken{}); err != nil {
		t.Fatalf("failed to create legacy token schema: %v", err)
	}
	if managedTokensTable != nil {
		*managedTokensTable = true
	}
	if err := db.Create(&legacyToken{
		UserId:             7,
		Key:                legacyKey,
		Status:             common.TokenStatusEnabled,
		Name:               "legacy-token",
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        100,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}).Error; err != nil {
		t.Fatalf("failed to seed legacy token row: %v", err)
	}

	if got := getTokenKeyColumnType(t, db, dialect); got != "char(48)" {
		t.Fatalf("expected legacy key column type char(48), got %q", got)
	}

	migrateTokenControllerTestDB(t, db)

	if got := getTokenKeyColumnType(t, db, dialect); got != "varchar(128)" {
		t.Fatalf("expected migrated key column type varchar(128), got %q", got)
	}

	var migratedToken model.Token
	if err := db.First(&migratedToken, "name = ?", "legacy-token").Error; err != nil {
		t.Fatalf("failed to load migrated token row: %v", err)
	}
	if migratedToken.Key != legacyKey {
		t.Fatalf("expected migrated token key %q, got %q", legacyKey, migratedToken.Key)
	}
	if migratedToken.Name != "legacy-token" {
		t.Fatalf("expected migrated token name to be preserved, got %q", migratedToken.Name)
	}

	inserted := model.Token{
		UserId:             8,
		Name:               "long-token",
		Key:                longKey,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        200,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}
	if err := db.Create(&inserted).Error; err != nil {
		t.Fatalf("failed to insert long token after migration: %v", err)
	}

	var fetched model.Token
	if err := db.First(&fetched, "id = ?", inserted.Id).Error; err != nil {
		t.Fatalf("failed to fetch long token after migration: %v", err)
	}
	if fetched.Key != longKey {
		t.Fatalf("expected long token key %q, got %q", longKey, fetched.Key)
	}
}

func TestTokenAutoMigrateUsesVarchar128KeyColumn(t *testing.T) {
	db := setupTokenControllerTestDB(t)

	if got := getTokenKeyColumnType(t, db, "sqlite"); got != "varchar(128)" {
		t.Fatalf("expected key column type varchar(128), got %q", got)
	}
}

func TestTokenMigrationFromChar48ToVarchar128(t *testing.T) {
	db := openTokenControllerTestDB(t)
	runTokenMigrationCompatibilityTest(t, db, "sqlite", nil)
}

func TestTokenMigrationFromChar48ToVarchar128MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run mysql migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "mysql", dsn)
	runTokenMigrationCompatibilityTest(t, db, "mysql", managedTokensTable)
}

func TestTokenMigrationFromChar48ToVarchar128Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "postgres", dsn)
	runTokenMigrationCompatibilityTest(t, db, "postgres", managedTokensTable)
}

func TestGetAllTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "list-token", "abcd1234efgh5678")
	seedToken(t, db, 2, "other-user-token", "zzzz1234yyyy5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 1)
	GetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode token page response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one token, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("list response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestSearchTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "searchable-token", "ijkl1234mnop5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/search?keyword=searchable-token&p=1&size=10", nil, 1)
	SearchTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one search result, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked search key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("search response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestGetTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "detail-token", "qrst1234uvwx5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token detail response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked detail key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("detail response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestUpdateTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "editable-token", "yzab1234cdef5678")
	require.NoError(t, db.AutoMigrate(&model.Ability{}))
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "mapped-target", ChannelId: 1, Enabled: true,
	}).Error)

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "updated-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"model_mapping":        `{"client-alias":"mapped-target"}`,
		"group":                "default",
		"cross_group_retry":    false,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token update response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked update key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	require.Equal(t, `{"client-alias":"mapped-target"}`, detail.ModelMapping)
	var reloaded model.Token
	require.NoError(t, db.First(&reloaded, token.Id).Error)
	require.Equal(t, `{"client-alias":"mapped-target"}`, reloaded.ModelMapping)
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("update response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestTokenModelMappingRejectsJSONNull(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       76,
		Username: "null-model-mapping-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	token := seedToken(t, db, 76, "null-model-mapping-token", "nullmapping123456")

	t.Run("create", func(t *testing.T) {
		ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
			"name":            "invalid-null-model-mapping",
			"expired_time":    -1,
			"unlimited_quota": true,
			"model_mapping":   "null",
		}, 76)
		AddToken(ctx)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		response := decodeAPIResponse(t, recorder)
		assert.False(t, response.Success)
		assert.Equal(t, "model mapping must be a valid JSON object", response.Message)

		var count int64
		require.NoError(t, db.Model(&model.Token{}).Where("name = ?", "invalid-null-model-mapping").Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("update", func(t *testing.T) {
		ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
			"id":                   token.Id,
			"name":                 "must-not-be-saved",
			"expired_time":         -1,
			"unlimited_quota":      true,
			"model_limits_enabled": false,
			"model_limits":         "",
			"model_mapping":        "null",
			"group":                "default",
		}, 76)
		UpdateToken(ctx)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		response := decodeAPIResponse(t, recorder)
		assert.False(t, response.Success)
		assert.Equal(t, "model mapping must be a valid JSON object", response.Message)

		var reloaded model.Token
		require.NoError(t, db.First(&reloaded, token.Id).Error)
		assert.Equal(t, "null-model-mapping-token", reloaded.Name)
		assert.Empty(t, reloaded.ModelMapping)
	})
}

func useTokenPricingGroups(t *testing.T) {
	t.Helper()

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1.5,"selectable":true},
		{"id":3,"name":"private","ratio":2,"selectable":false}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["1","2"]`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))
}

func TestAddTokenDefaultsToAutoAndNormalizesCandidateIDs(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       71,
		Username: "auto-token-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":                  "auto-token",
		"expired_time":          -1,
		"unlimited_quota":       true,
		"auto_group_candidates": []string{"vip", "2", "default"},
		"cross_group_retry":     true,
	}, 71)
	AddToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, decodeAPIResponse(t, recorder).Success)

	var stored model.Token
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&stored, "name = ?", "auto-token").Error)
	assert.Equal(t, "auto", stored.Group)
	assert.Equal(t, model.PricingGroupCandidates("2,1"), stored.AutoGroupCandidates)
	assert.False(t, stored.CrossGroupRetry)

	getCtx, getRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(stored.Id), nil, 71)
	getCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(stored.Id)}}
	GetToken(getCtx)

	var detail tokenResponseItem
	response := decodeAPIResponse(t, getRecorder)
	require.True(t, response.Success)
	require.NoError(t, common.Unmarshal(response.Data, &detail))
	assert.Equal(t, "auto", detail.Group)
	assert.Equal(t, []string{"2", "1"}, detail.AutoGroupCandidates)
	assert.NotContains(t, getRecorder.Body.String(), "cross_group_retry")
}

func TestAddTokenReturnsCreatedIDAndMaskedKey(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       75,
		Username: "created-token-response-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":            "created-token-response",
		"expired_time":    -1,
		"unlimited_quota": true,
	}, 75)
	AddToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	var created tokenResponseItem
	require.NoError(t, common.Unmarshal(response.Data, &created))
	require.NotZero(t, created.ID)
	require.NotEmpty(t, created.Key)

	var stored model.Token
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&stored, created.ID).Error)
	assert.Equal(t, stored.GetMaskedKey(), created.Key)
	assert.NotEqual(t, stored.Key, created.Key)
	assert.NotContains(t, recorder.Body.String(), stored.Key)
}

func TestCreateOnboardingTokenCreatesExactlyOneKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       81,
		Username: "onboarding-token-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	var wg sync.WaitGroup
	created := make(chan bool, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			didCreate, err := model.CreateOnboardingToken(81, "auto", "")
			created <- didCreate
			errs <- err
		}()
	}
	wg.Wait()
	close(created)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	createdCount := 0
	for didCreate := range created {
		if didCreate {
			createdCount++
		}
	}
	assert.Equal(t, 1, createdCount)

	var tokens []model.Token
	require.NoError(t, db.Where("user_id = ?", 81).Find(&tokens).Error)
	require.Len(t, tokens, 1)
	assert.Equal(t, "Мой первый ключ", tokens[0].Name)
	assert.Equal(t, "auto", tokens[0].Group)
	assert.True(t, tokens[0].UnlimitedQuota)
	assert.Equal(t, int64(-1), tokens[0].ExpiredTime)
}

func TestCreateOnboardingTokenRespectsMaxUserTokens(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       82,
		Username: "onboarding-limit-user",
		Password: "not-used-in-test",
		Status:   common.UserStatusEnabled,
	}).Error)

	settings := operation_setting.GetTokenSetting()
	originalMax := settings.MaxUserTokens
	settings.MaxUserTokens = 0
	t.Cleanup(func() { settings.MaxUserTokens = originalMax })

	created, err := model.CreateOnboardingToken(82, "auto", "")
	require.ErrorIs(t, err, model.ErrUserTokenLimitReached)
	assert.False(t, created)

	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 82).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateOnboardingTokenSkipsUserWithRegularToken(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       83,
		Username: "regular-token-user",
		Password: "not-used-in-test",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, (&model.Token{
		UserId:       83,
		Name:         "regular",
		Key:          "regular-token-key",
		CreatedTime:  common.GetTimestamp(),
		AccessedTime: common.GetTimestamp(),
		ExpiredTime:  -1,
		Group:        "auto",
	}).Insert())

	created, err := model.CreateOnboardingToken(83, "auto", "")
	require.NoError(t, err)
	assert.False(t, created)

	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 83).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestOnboardingAndRegularTokenShareLimitTransaction(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       84,
		Username: "onboarding-race-user",
		Password: "not-used-in-test",
		Status:   common.UserStatusEnabled,
	}).Error)

	regular := &model.Token{
		UserId:       84,
		Name:         "regular",
		Key:          "onboarding-race-regular-key",
		CreatedTime:  common.GetTimestamp(),
		AccessedTime: common.GetTimestamp(),
		ExpiredTime:  -1,
		Group:        "auto",
	}
	var wg sync.WaitGroup
	regularResult := make(chan error, 1)
	onboardingResult := make(chan struct {
		created bool
		err     error
	}, 1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		regularResult <- regular.InsertWithUserTokenLimit(1)
	}()
	go func() {
		defer wg.Done()
		created, err := model.CreateOnboardingToken(84, "auto", "")
		onboardingResult <- struct {
			created bool
			err     error
		}{created, err}
	}()
	wg.Wait()

	regularErr := <-regularResult
	onboarding := <-onboardingResult
	require.True(t,
		regularErr == nil || errors.Is(regularErr, model.ErrUserTokenLimitReached),
		"unexpected regular token error: %v", regularErr,
	)
	require.NoError(t, onboarding.err)

	var tokens []model.Token
	require.NoError(t, db.Where("user_id = ?", 84).Find(&tokens).Error)
	require.Len(t, tokens, 1)
	assert.Equal(t, onboarding.created, tokens[0].Name == "Мой первый ключ")
}

func TestAddTokenFixedGroupClearsCandidates(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       72,
		Username: "fixed-token-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":                  "fixed-token",
		"expired_time":          -1,
		"unlimited_quota":       true,
		"group":                 "vip",
		"auto_group_candidates": []string{"private"},
	}, 72)
	AddToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, decodeAPIResponse(t, recorder).Success)

	var stored model.Token
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&stored, "name = ?", "fixed-token").Error)
	assert.Equal(t, "2", stored.Group)
	assert.Empty(t, stored.AutoGroupCandidates)
}

func TestAddTokenRejectsUnavailableExplicitAutoCandidate(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       73,
		Username: "candidate-token-user",
		Password: "not-used-in-test",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":                  "invalid-auto-token",
		"expired_time":          -1,
		"unlimited_quota":       true,
		"group":                 "auto",
		"auto_group_candidates": []string{"private"},
	}, 73)
	AddToken(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.False(t, decodeAPIResponse(t, recorder).Success)

	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("name = ?", "invalid-auto-token").Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateTokenDistinguishesOmittedAndEmptyAutoCandidates(t *testing.T) {
	useTokenPricingGroups(t)
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 74, "candidate-update-token", "updatecandidate1234")
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"group":                 "auto",
		"auto_group_candidates": "1,2",
		"cross_group_retry":     true,
	}).Error)

	baseBody := map[string]any{
		"id":                   token.Id,
		"name":                 "candidate-update-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "auto",
		"cross_group_retry":    false,
	}
	omittedCtx, omittedRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", baseBody, 74)
	UpdateToken(omittedCtx)
	require.Equal(t, http.StatusOK, omittedRecorder.Code)
	require.True(t, decodeAPIResponse(t, omittedRecorder).Success)

	var stored model.Token
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&stored, token.Id).Error)
	assert.Equal(t, model.PricingGroupCandidates("1,2"), stored.AutoGroupCandidates)
	assert.True(t, stored.CrossGroupRetry)
	assert.NotContains(t, omittedRecorder.Body.String(), "cross_group_retry")

	baseBody["auto_group_candidates"] = []string{}
	emptyCtx, emptyRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", baseBody, 74)
	UpdateToken(emptyCtx)
	require.Equal(t, http.StatusOK, emptyRecorder.Code)
	require.True(t, decodeAPIResponse(t, emptyRecorder).Success)

	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&stored, token.Id).Error)
	assert.Empty(t, stored.AutoGroupCandidates)
}

func TestGetTokenKeyRequiresOwnershipAndReturnsFullKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "owned-token", "owner1234token5678")

	authorizedCtx, authorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 1)
	authorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(authorizedCtx)

	authorizedResponse := decodeAPIResponse(t, authorizedRecorder)
	if !authorizedResponse.Success {
		t.Fatalf("expected authorized key fetch to succeed, got message: %s", authorizedResponse.Message)
	}

	var keyData tokenKeyResponse
	if err := common.Unmarshal(authorizedResponse.Data, &keyData); err != nil {
		t.Fatalf("failed to decode token key response: %v", err)
	}
	if keyData.Key != token.GetFullKey() {
		t.Fatalf("expected full key %q, got %q", token.GetFullKey(), keyData.Key)
	}

	unauthorizedCtx, unauthorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 2)
	unauthorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(unauthorizedCtx)

	unauthorizedResponse := decodeAPIResponse(t, unauthorizedRecorder)
	if unauthorizedResponse.Success {
		t.Fatalf("expected unauthorized key fetch to fail")
	}
	if strings.Contains(unauthorizedRecorder.Body.String(), token.Key) {
		t.Fatalf("unauthorized key response leaked raw token key: %s", unauthorizedRecorder.Body.String())
	}
}
