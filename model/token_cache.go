package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/go-redis/redis/v8"
)

func tokenAdminDisabledCacheKey(key string) string {
	return "token-admin-disabled:" + common.GenerateHMAC(key)
}

func cacheSetToken(token Token) error {
	key := common.GenerateHMAC(token.Key)
	token.NormalizeRouting()
	token.Clean()
	err := common.RedisHSetObj(fmt.Sprintf("token:%s", key), &token, time.Duration(common.RedisKeyCacheSeconds())*time.Second)
	if err != nil {
		return err
	}
	return nil
}

func cacheDeleteToken(key string) error {
	// Retain administrative block markers as revocation tombstones: an older
	// asynchronous cache fill may still recreate the deleted token's cache.
	key = common.GenerateHMAC(key)
	err := common.RedisDelKey(fmt.Sprintf("token:%s", key))
	if err != nil {
		return err
	}
	return nil
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	hmacKey := common.GenerateHMAC(key)
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(
		fmt.Sprintf("token:%s", hmacKey),
		&token,
		constant.TokenFieldAutoGroupCandidates,
	)
	if err != nil {
		return nil, err
	}
	token.Key = key
	token.NormalizeRouting()
	_, err = common.RedisGet(tokenAdminDisabledCacheKey(key))
	if err == nil {
		token.Status = common.TokenStatusAdminDisabled
	} else if !errors.Is(err, redis.Nil) {
		return nil, err
	} else if token.Status == common.TokenStatusAdminDisabled {
		// An older asynchronous cache fill may finish after an administrator unblocks the key.
		return nil, fmt.Errorf("administrative token status needs database refresh")
	}
	return &token, nil
}
