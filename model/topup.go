package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	RequestedAmount float64 `json:"requested_amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	QuotaToAdd      int     `json:"quota_to_add"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodYooKassaSBP  = "yookassa_sbp"
	PaymentMethodNOWPayments  = "nowpayments"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderYooKassa     = "yookassa"
	PaymentProviderNOWPayments  = "nowpayments"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

type topUpCompletionPrepare func(*TopUp) (map[string]interface{}, error)
type topUpCompletionApply func(*gorm.DB, *TopUp) error

func completeTopUpCAS(tradeNo, expectedProvider string, prepare topUpCompletionPrepare, apply topUpCompletionApply) (*TopUp, bool, error) {
	topUp := &TopUp{}
	if err := DB.Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
		return nil, false, ErrTopUpNotFound
	}
	if expectedProvider != "" && topUp.PaymentProvider != expectedProvider {
		return nil, false, ErrPaymentMethodMismatch
	}
	if topUp.Status == common.TopUpStatusSuccess {
		return topUp, false, nil
	}
	if topUp.Status != common.TopUpStatusPending {
		return nil, false, ErrTopUpStatusInvalid
	}

	updates, err := prepare(topUp)
	if err != nil {
		return nil, false, err
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	completeTime := common.GetTimestamp()
	updates["complete_time"] = completeTime
	updates["status"] = common.TopUpStatusSuccess

	providerGuard := topUp.PaymentProvider
	casLost := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&TopUp{}).
			Where("id = ? AND payment_provider = ? AND status = ?", topUp.Id, providerGuard, common.TopUpStatusPending).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			casLost = true
			return nil
		}
		return apply(tx, topUp)
	})
	if err != nil {
		return nil, false, err
	}
	if casLost {
		current := &TopUp{}
		if err := DB.Where("id = ?", topUp.Id).First(current).Error; err != nil {
			return nil, false, err
		}
		if current.PaymentProvider != providerGuard {
			return nil, false, ErrPaymentMethodMismatch
		}
		if current.Status == common.TopUpStatusSuccess {
			return current, false, nil
		}
		return nil, false, ErrTopUpStatusInvalid
	}

	topUp.CompleteTime = completeTime
	topUp.Status = common.TopUpStatusSuccess
	return topUp, true, nil
}

func resolveTopUpQuota(topUp *TopUp) (int, error) {
	if topUp.QuotaToAdd > 0 {
		return topUp.QuotaToAdd, nil
	}

	switch topUp.PaymentProvider {
	case PaymentProviderCreem:
		if topUp.Amount > 0 {
			return int(topUp.Amount), nil
		}
	case PaymentProviderStripe:
		quota := int(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quota > 0 {
			return quota, nil
		}
	case PaymentProviderYooKassa:
		paymentMetadata := GetPaymentMetadataByTradeNo(topUp.TradeNo)
		if paymentMetadata == nil || paymentMetadata.PaymentProvider != PaymentProviderYooKassa {
			break
		}
		var metadata struct {
			QuotaToAdd string `json:"quota_to_add"`
		}
		if err := common.Unmarshal([]byte(paymentMetadata.Metadata), &metadata); err != nil {
			break
		}
		quota, err := strconv.Atoi(metadata.QuotaToAdd)
		if err == nil && quota > 0 {
			return quota, nil
		}
	case PaymentProviderNOWPayments:
		// NOWPayments has persisted QuotaToAdd since its first release.
		break
	default:
		quota := int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quota > 0 {
			return quota, nil
		}
	}

	return 0, errors.New("无效或不明确的充值额度")
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	topUp, completed, err := completeTopUpCAS(referenceId, PaymentProviderStripe, func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuota(topUp)
		quota = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)
	}

	return nil
}

func RechargeEpay(tradeNo, paymentMethod, callerIP string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var quotaCredited bool
	var casLost bool
	topUp := &TopUp{}

	if err := DB.Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
		return errors.New("充值订单不存在")
	}
	if topUp.PaymentProvider != PaymentProviderEpay {
		return ErrPaymentMethodMismatch
	}
	if topUp.Status == common.TopUpStatusSuccess {
		return nil
	}
	if topUp.Status != common.TopUpStatusPending {
		return ErrTopUpStatusInvalid
	}

	quotaToAdd = topUp.QuotaToAdd
	if quotaToAdd <= 0 {
		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
	}
	if quotaToAdd <= 0 {
		return errors.New("无效的充值额度")
	}

	if paymentMethod != "" {
		topUp.PaymentMethod = paymentMethod
	}
	topUp.CompleteTime = common.GetTimestamp()
	topUp.Status = common.TopUpStatusSuccess

	err = DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"complete_time": topUp.CompleteTime,
			"status":        common.TopUpStatusSuccess,
		}
		if paymentMethod != "" {
			updates["payment_method"] = paymentMethod
		}
		statusResult := tx.Model(&TopUp{}).
			Where("id = ? AND payment_provider = ? AND status = ?", topUp.Id, PaymentProviderEpay, common.TopUpStatusPending).
			Updates(updates)
		if statusResult.Error != nil {
			return statusResult.Error
		}
		if statusResult.RowsAffected == 0 {
			casLost = true
			return nil
		}
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		quotaCredited = true
		return nil
	})
	if err != nil {
		common.SysError("epay topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if casLost {
		current := &TopUp{}
		if err := DB.Where("id = ?", topUp.Id).First(current).Error; err != nil {
			return errors.New("充值失败，请稍后重试")
		}
		if current.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if current.Status == common.TopUpStatusSuccess {
			return nil
		}
		return ErrTopUpStatusInvalid
	}
	if quotaCredited {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIP, topUp.PaymentMethod, PaymentProviderEpay)
	}
	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	var quotaToAdd int
	topUp, completed, err := completeTopUpCAS(tradeNo, "", func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuota(topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, "admin")
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp, completed, err := completeTopUpCAS(referenceId, PaymentProviderCreem, func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, err := resolveTopUpQuota(topUp)
		quota = int64(resolvedQuota)
		return nil, err
	}, func(tx *gorm.DB, topUp *TopUp) error {
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}
		if customerEmail != "" {
			var user User
			if err := tx.Where("id = ?", topUp.UserId).First(&user).Error; err != nil {
				return err
			}
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		return nil
	})
	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)
	}

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp, completed, err := completeTopUpCAS(tradeNo, PaymentProviderWaffo, func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuota(topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		return nil
	})
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp, completed, err := completeTopUpCAS(tradeNo, PaymentProviderWaffoPancake, func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuota(topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("充值用户不存在")
		}
		return nil
	})
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completed {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}

func RechargeYooKassa(tradeNo string, callerIp string) (err error) {
	return rechargeProviderTopUp(tradeNo, callerIp, PaymentProviderYooKassa, "YooKassa")
}

func RechargeNOWPayments(tradeNo string, callerIp string) (err error) {
	return rechargeProviderTopUp(tradeNo, callerIp, PaymentProviderNOWPayments, "NOWPayments")
}

func rechargeProviderTopUp(tradeNo string, callerIp, provider, providerName string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp, completed, err := completeTopUpCAS(tradeNo, provider, func(topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuota(topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("Top-up user does not exist")
		}
		return nil
	})
	if err != nil {
		common.SysError(strings.ToLower(providerName) + " topup failed: " + err.Error())
		return errors.New("Top-up failed, please try again later")
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("%s top-up succeeded, quota: %v, payment amount: %.2f", providerName, logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, provider)
	}

	return nil
}
