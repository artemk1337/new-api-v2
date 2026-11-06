package model

import "github.com/QuantumNous/new-api/common"

// SetTokenAdminStatus changes only the status and refreshes the shared auth cache.
func SetTokenAdminStatus(token *Token, status int) error {
	// Publish the block first: a Redis failure must not leave a disabled DB row
	// behind an enabled cache. A subsequent DB failure remains fail-closed.
	if common.RedisEnabled && status == common.TokenStatusAdminDisabled {
		if err := common.RedisSet(tokenAdminDisabledCacheKey(token.Key), "1", 0); err != nil {
			return err
		}
	}
	if err := DB.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId).Update("status", status).Error; err != nil {
		return err
	}
	token.Status = status
	if common.RedisEnabled {
		if err := cacheSetToken(*token); err != nil {
			return err
		}
		if status != common.TokenStatusAdminDisabled {
			return common.RedisDelKey(tokenAdminDisabledCacheKey(token.Key))
		}
	}
	return nil
}
