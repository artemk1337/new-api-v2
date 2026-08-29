package model

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPricingGroupCandidatesJSONContract(t *testing.T) {
	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1.5,"selectable":true}
	]`))

	token := Token{
		Group:               "auto",
		AutoGroupCandidates: NewPricingGroupCandidates([]string{"vip", "2", "default", "vip"}),
	}
	token.NormalizeRouting()

	payload, err := common.Marshal(token)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"auto_group_candidates":["2","1"]`)

	var decoded Token
	require.NoError(t, common.Unmarshal([]byte(`{
		"group":"auto",
		"auto_group_candidates":["vip","2","default"]
	}`), &decoded))
	decoded.NormalizeRouting()
	assert.Equal(t, PricingGroupCandidates("2,1"), decoded.AutoGroupCandidates)
	assert.Equal(t, []string{"2", "1"}, decoded.GetAutoGroupCandidates())
}

func TestTokenCreateDefaultsGroupToAuto(t *testing.T) {
	truncateTables(t)

	token := &Token{
		UserId: 1,
		Key:    "default-auto-token",
		Name:   "default auto",
	}
	require.NoError(t, DB.Create(token).Error)

	var stored Token
	require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).First(&stored, token.Id).Error)
	assert.Equal(t, "auto", stored.Group)
	assert.Empty(t, stored.AutoGroupCandidates)
}

func TestMigrateLegacyTokenGroupsToAutoIsIdempotent(t *testing.T) {
	truncateTables(t)

	emptyGroup := &Token{UserId: 1, Key: "legacy-empty-token", Name: "empty"}
	nullGroup := &Token{UserId: 1, Key: "legacy-null-token", Name: "null"}
	fixedGroup := &Token{UserId: 1, Key: "legacy-fixed-token", Name: "fixed", Group: "default"}
	require.NoError(t, DB.Create(emptyGroup).Error)
	require.NoError(t, DB.Create(nullGroup).Error)
	require.NoError(t, DB.Create(fixedGroup).Error)

	require.NoError(t, DB.Exec("UPDATE tokens SET "+commonGroupCol+" = ? WHERE id = ?", "", emptyGroup.Id).Error)
	require.NoError(t, DB.Exec("UPDATE tokens SET "+commonGroupCol+" = NULL WHERE id = ?", nullGroup.Id).Error)
	require.NoError(t, migrateLegacyTokenGroupsToAuto())
	require.NoError(t, migrateLegacyTokenGroupsToAuto())

	var stored []Token
	require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).Order("id").Find(&stored).Error)
	require.Len(t, stored, 3)
	assert.Equal(t, "auto", stored[0].Group)
	assert.Equal(t, "auto", stored[1].Group)
	assert.Equal(t, "1", stored[2].Group)
}

func TestTokenCacheRequiresAutoGroupCandidatesField(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redisClient
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
		require.NoError(t, redisClient.Close())
	})

	const tokenKey = "legacy-routing-cache"
	require.NoError(t, cacheSetToken(Token{
		Key:                 tokenKey,
		Group:               "auto",
		AutoGroupCandidates: NewPricingGroupCandidates([]string{"2"}),
	}))
	cacheKey := fmt.Sprintf("token:%s", common.GenerateHMAC(tokenKey))
	require.NoError(t, common.RDB.HDel(
		context.Background(),
		cacheKey,
		constant.TokenFieldAutoGroupCandidates,
	).Err())

	_, err := cacheGetTokenByKey(tokenKey)
	require.ErrorContains(t, err, "missing required field AutoGroupCandidates")
}

func TestTokenCacheAcceptsPresentEmptyAutoGroupCandidates(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redisClient
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
		require.NoError(t, redisClient.Close())
	})

	const tokenKey = "all-groups-routing-cache"
	require.NoError(t, cacheSetToken(Token{
		Key:   tokenKey,
		Group: "auto",
	}))

	cached, err := cacheGetTokenByKey(tokenKey)
	require.NoError(t, err)
	assert.Empty(t, cached.GetAutoGroupCandidates())
	cacheKey := fmt.Sprintf("token:%s", common.GenerateHMAC(tokenKey))
	exists, err := common.RDB.HExists(
		context.Background(),
		cacheKey,
		constant.TokenFieldAutoGroupCandidates,
	).Result()
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTokenCachePreservesModelMapping(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redisClient
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
		require.NoError(t, redisClient.Close())
	})

	const tokenKey = "mapped-token-cache"
	require.NoError(t, cacheSetToken(Token{
		Key:          tokenKey,
		Group:        "auto",
		ModelMapping: `{"client-alias":"provider-model"}`,
	}))

	cached, err := cacheGetTokenByKey(tokenKey)
	require.NoError(t, err)
	assert.Equal(t, `{"client-alias":"provider-model"}`, cached.GetModelMapping())
}

func TestTokenWritesBeforeAutoGroupCandidatesMigrationAreFailClosed(t *testing.T) {
	legacyDB, err := gorm.Open(
		sqlite.Open(filepath.Join(t.TempDir(), "legacy-token-schema.db")),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, legacyDB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, legacyDB.Migrator().DropColumn(&Token{}, "auto_group_candidates"))
	require.False(t, legacyDB.Migrator().HasColumn(&Token{}, "auto_group_candidates"))
	require.NoError(t, legacyDB.Create(&User{Id: 1, Username: "legacy-routing-user"}).Error)

	previousDB := DB
	DB = legacyDB
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, dbErr := legacyDB.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	allGroups := &Token{
		UserId: 1,
		Key:    "pre-migration-all-groups",
		Name:   "all groups",
		Group:  "auto",
	}
	require.NoError(t, allGroups.Insert())
	allGroups.Name = "all groups updated"
	require.NoError(t, allGroups.Update())

	subset := &Token{
		UserId:              1,
		Key:                 "pre-migration-subset",
		Name:                "subset",
		Group:               "auto",
		AutoGroupCandidates: NewPricingGroupCandidates([]string{"2"}),
	}
	err = subset.Insert()
	require.ErrorIs(t, err, ErrTokenRoutingMigrationPending)
	assert.Contains(t, err.Error(), "select all groups or try again shortly")

	var subsetCount int64
	require.NoError(t, legacyDB.Model(&Token{}).
		Where(commonKeyCol+" = ?", subset.Key).
		Count(&subsetCount).Error)
	assert.Zero(t, subsetCount)

	allGroups.AutoGroupCandidates = NewPricingGroupCandidates([]string{"2"})
	allGroups.Name = "must not be persisted"
	err = allGroups.Update()
	require.ErrorIs(t, err, ErrTokenRoutingMigrationPending)

	var storedName string
	require.NoError(t, legacyDB.Model(&Token{}).
		Select("name").
		Where("id = ?", allGroups.Id).
		Scan(&storedName).Error)
	assert.Equal(t, "all groups updated", storedName)
}

func TestTokenWritesBeforeModelMappingMigrationRemainCompatible(t *testing.T) {
	legacyDB, err := gorm.Open(
		sqlite.Open(filepath.Join(t.TempDir(), "legacy-token-model-mapping-schema.db")),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, legacyDB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, legacyDB.Migrator().DropColumn(&Token{}, "model_mapping"))
	require.False(t, legacyDB.Migrator().HasColumn(&Token{}, "model_mapping"))
	require.NoError(t, legacyDB.Create(&User{Id: 1, Username: "legacy-model-mapping-user"}).Error)

	previousDB := DB
	DB = legacyDB
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, dbErr := legacyDB.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	token := &Token{
		UserId: 1,
		Key:    "pre-migration-empty-model-mapping",
		Name:   "empty mapping",
		Group:  "auto",
	}
	require.NoError(t, token.InsertWithUserTokenLimit(2))
	token.Name = "empty mapping updated"
	require.NoError(t, token.Update())

	fetched, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	assert.Equal(t, token.Id, fetched.Id)
	assert.Empty(t, fetched.ModelMapping)

	listed, err := GetAllUserTokens(token.UserId, 0, 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, token.Id, listed[0].Id)

	mapped := &Token{
		UserId:       1,
		Key:          "pre-migration-mapped-token",
		Name:         "mapped token",
		Group:        "auto",
		ModelMapping: `{"client-alias":"provider-model"}`,
	}
	err = mapped.InsertWithUserTokenLimit(2)
	require.ErrorIs(t, err, ErrTokenModelMappingMigrationPending)
	assert.Contains(t, err.Error(), "Model mapping is temporarily unavailable")

	var mappedCount int64
	require.NoError(t, legacyDB.Model(&Token{}).
		Where(commonKeyCol+" = ?", mapped.Key).
		Count(&mappedCount).Error)
	assert.Zero(t, mappedCount)

	token.Name = "must not be persisted"
	token.ModelMapping = `{"client-alias":"provider-model"}`
	err = token.Update()
	require.ErrorIs(t, err, ErrTokenModelMappingMigrationPending)

	var storedName string
	require.NoError(t, legacyDB.Model(&Token{}).
		Select("name").
		Where("id = ?", token.Id).
		Scan(&storedName).Error)
	assert.Equal(t, "empty mapping updated", storedName)
}

func TestCreateOnboardingTokenBeforeRoutingMigrationsRemainsCompatible(t *testing.T) {
	legacyDB, err := gorm.Open(
		sqlite.Open(filepath.Join(t.TempDir(), "legacy-onboarding-token-schema.db")),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, legacyDB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, legacyDB.Migrator().DropColumn(&Token{}, "model_mapping"))
	require.NoError(t, legacyDB.Migrator().DropColumn(&Token{}, "auto_group_candidates"))
	require.NoError(t, legacyDB.Create(&User{Id: 1, Username: "legacy-onboarding-user", AffCode: "legacy-onboarding-1"}).Error)
	require.NoError(t, legacyDB.Create(&User{Id: 2, Username: "legacy-onboarding-subset-user", AffCode: "legacy-onboarding-2"}).Error)

	previousDB := DB
	DB = legacyDB
	t.Cleanup(func() {
		DB = previousDB
		refreshTokenModelMappingColumnCache()
		sqlDB, dbErr := legacyDB.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	created, err := CreateOnboardingToken(1, "auto", "")
	require.NoError(t, err)
	assert.True(t, created)

	var stored struct {
		Group string
	}
	require.NoError(t, legacyDB.Model(&Token{}).Select("group").Where("user_id = ?", 1).First(&stored).Error)
	assert.Equal(t, "auto", stored.Group)

	created, err = CreateOnboardingToken(2, "auto", NewPricingGroupCandidates([]string{"default"}))
	require.ErrorIs(t, err, ErrTokenRoutingMigrationPending)
	assert.False(t, created)
	var count int64
	require.NoError(t, legacyDB.Model(&Token{}).Where("user_id = ?", 2).Count(&count).Error)
	assert.Zero(t, count)
}

func TestLegacyTokenModelMappingSchemaSupportsRefundAndNormalization(t *testing.T) {
	legacyDB, err := gorm.Open(
		sqlite.Open(filepath.Join(t.TempDir(), "legacy-token-model-mapping-refund.db")),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, legacyDB.AutoMigrate(&Token{}, &Task{}, &BillingOutbox{}))
	require.NoError(t, legacyDB.Migrator().DropColumn(&Token{}, "model_mapping"))

	previousDB := DB
	DB = legacyDB
	t.Cleanup(func() {
		DB = previousDB
		refreshTokenModelMappingColumnCache()
		sqlDB, dbErr := legacyDB.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	token := Token{Key: "legacy-refund-token", Group: "auto", RemainQuota: 10, UsedQuota: 10}
	require.NoError(t, legacyDB.Omit("model_mapping").Create(&token).Error)
	task := Task{Quota: 3, PrivateData: TaskPrivateData{TokenId: token.Id}}
	require.NoError(t, legacyDB.Create(&task).Error)
	event := BillingOutbox{
		EventID:     "legacy-refund-event",
		Kind:        BillingOutboxKindTaskRefund,
		ReferenceID: fmt.Sprint(task.ID),
		Stage:       billingOutboxStageFundingDone,
	}
	require.NoError(t, legacyDB.Create(&event).Error)

	require.NoError(t, refundTaskToken(&event))
	require.NoError(t, normalizeTokenPricingGroupsTx(legacyDB))

	var stored Token
	require.NoError(t, tokenReadDB(legacyDB).First(&stored, token.Id).Error)
	assert.Equal(t, 13, stored.RemainQuota)
	assert.Equal(t, 7, stored.UsedQuota)

	require.NoError(t, legacyDB.AutoMigrate(&Token{}))
	refreshTokenModelMappingColumnCache()
	assert.True(t, hasTokenModelMappingColumn())
}
