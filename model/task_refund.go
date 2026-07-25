package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// refundTaskFundingSources returns a failed async task charge to the exact
// sources captured when the task was created. It must be called inside the
// same transaction that advances the durable refund stage.
func refundTaskFundingSources(tx *gorm.DB, task *Task) (int, error) {
	if task.Quota < 0 {
		return 0, errors.New("task refund quota cannot be negative")
	}
	if task.Quota == 0 {
		return 0, nil
	}

	walletRefund := task.Quota
	subscriptionRefund := 0
	if task.PrivateData.BillingSource == "subscription" {
		if task.PrivateData.SubscriptionId <= 0 {
			return 0, errors.New("subscription task refund is missing subscription id")
		}
		walletRefund = 0
		subscriptionRefund = task.Quota
		if task.PrivateData.BillingOverageSource != "" {
			if task.PrivateData.BillingOverageSource != "wallet" {
				return 0, fmt.Errorf("unsupported task billing overage source %q", task.PrivateData.BillingOverageSource)
			}
			walletRefund = task.PrivateData.BillingOverageQuota
			if walletRefund < 0 || walletRefund > task.Quota {
				return 0, fmt.Errorf("invalid task billing overage quota %d for total quota %d", walletRefund, task.Quota)
			}
			subscriptionRefund -= walletRefund
		}
	}

	if subscriptionRefund > 0 {
		if err := refundUserSubscriptionQuotaForBillingTx(tx, task.PrivateData.SubscriptionId, subscriptionRefund); err != nil {
			return 0, err
		}
	}

	if walletRefund > 0 {
		result := tx.Model(&User{}).Where("id = ?", task.UserId).
			Update("quota", gorm.Expr("quota + ?", walletRefund))
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected == 0 {
			return 0, gorm.ErrRecordNotFound
		}
	}
	return walletRefund, nil
}

func refundUserSubscriptionQuotaForBillingTx(tx *gorm.DB, subscriptionID int, quota int) error {
	if subscriptionID <= 0 {
		return errors.New("invalid subscription id")
	}
	if quota < 0 {
		return errors.New("subscription refund quota cannot be negative")
	}
	if quota == 0 {
		return nil
	}
	result := tx.Model(&UserSubscription{}).
		Where("id = ? AND amount_used >= ?", subscriptionID, quota).
		Update("amount_used", gorm.Expr("amount_used - ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("subscription refund source missing or insufficient: id=%d quota=%d", subscriptionID, quota)
	}
	return nil
}

// RefundTaskFundingSources is the synchronous compatibility path. Durable
// terminal refunds call refundTaskFundingSources inside their outbox-stage
// transaction instead.
func RefundTaskFundingSources(task *Task) error {
	walletRefund := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		walletRefund, err = refundTaskFundingSources(tx, task)
		return err
	})
	if err == nil && walletRefund > 0 {
		invalidateUserQuotaAfterBilling(task.UserId)
	}
	return err
}

func RefundUserSubscriptionQuotaForBilling(subscriptionID int, quota int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return refundUserSubscriptionQuotaForBillingTx(tx, subscriptionID, quota)
	})
}
