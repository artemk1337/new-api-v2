package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var (
	ErrBillingUserQuotaInsufficient  = errors.New("user quota is insufficient")
	ErrBillingTokenQuotaInsufficient = errors.New("token quota is insufficient")
)

// ReserveUserQuotaForBilling atomically reserves wallet quota without allowing
// the balance to become negative. Billing writes always go directly to the
// database and synchronously invalidate the Redis snapshot afterwards.
func ReserveUserQuotaForBilling(userID int, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}

	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", userID, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	invalidateUserQuotaAfterBilling(userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var available int
		err := DB.Model(&User{}).Where("id = ?", userID).Select("quota").Scan(&available).Error
		if err != nil {
			return errors.Join(ErrBillingUserQuotaInsufficient, err)
		}
		return fmt.Errorf("%w: available=%d required=%d", ErrBillingUserQuotaInsufficient, available, quota)
	}
	return nil
}

// DebitUserQuotaForBilling records a post-dispatch wallet charge. Unlike a
// reserve, the debit is allowed to make the balance negative.
func DebitUserQuotaForBilling(userID int, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return adjustUserQuotaForBilling(userID, -quota)
}

func RefundUserQuotaForBilling(userID int, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return adjustUserQuotaForBilling(userID, quota)
}

func adjustUserQuotaForBilling(userID int, delta int) error {
	if delta == 0 {
		return nil
	}

	result := DB.Model(&User{}).
		Where("id = ?", userID).
		Update("quota", gorm.Expr("quota + ?", delta))
	invalidateUserQuotaAfterBilling(userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ReserveTokenQuotaForBilling atomically reserves an API key quota. Unlimited
// keys keep the historical behavior and may have a negative remain_quota.
func ReserveTokenQuotaForBilling(tokenID int, key string, quota int, unlimited bool) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}

	query := DB.Model(&Token{}).Where("id = ?", tokenID)
	if !unlimited {
		query = query.Where("remain_quota >= ?", quota)
	}
	result := query.Updates(map[string]interface{}{
		"remain_quota":  gorm.Expr("remain_quota - ?", quota),
		"used_quota":    gorm.Expr("used_quota + ?", quota),
		"accessed_time": common.GetTimestamp(),
	})
	invalidateTokenQuotaAfterBilling(key)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var token Token
		err := DB.Select("remain_quota").Where("id = ?", tokenID).First(&token).Error
		if err != nil {
			return errors.Join(ErrBillingTokenQuotaInsufficient, err)
		}
		return fmt.Errorf("%w: available=%d required=%d", ErrBillingTokenQuotaInsufficient, token.RemainQuota, quota)
	}
	return nil
}

// DebitTokenQuotaForBilling records a post-dispatch API key charge and may
// make remain_quota negative.
func DebitTokenQuotaForBilling(tokenID int, key string, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return adjustTokenQuotaForBilling(tokenID, key, -quota)
}

func RefundTokenQuotaForBilling(tokenID int, key string, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return adjustTokenQuotaForBilling(tokenID, key, quota)
}

func adjustTokenQuotaForBilling(tokenID int, key string, delta int) error {
	if delta == 0 {
		return nil
	}

	result := DB.Model(&Token{}).
		Where("id = ?", tokenID).
		Updates(map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		})
	invalidateTokenQuotaAfterBilling(key)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func invalidateUserQuotaAfterBilling(userID int) {
	if err := invalidateUserCache(userID); err != nil {
		common.SysLog("failed to invalidate user quota cache after billing write: " + err.Error())
	}
}

func invalidateTokenQuotaAfterBilling(key string) {
	if !common.RedisEnabled || key == "" {
		return
	}
	if err := cacheDeleteToken(key); err != nil {
		common.SysLog("failed to invalidate token quota cache after billing write: " + err.Error())
	}
}
