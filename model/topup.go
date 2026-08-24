package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

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
	// PaymentMethodName is the public label captured for gateways with multiple
	// user-facing methods (for example Waffo Card/Apple Pay).
	PaymentMethodName string `json:"payment_method_name,omitempty" gorm:"type:varchar(128);default:''"`
	PaymentProvider   string `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	// CreemProductID is the immutable catalog product selected at checkout.
	// It is checked against every settlement callback.
	CreemProductID string `json:"creem_product_id,omitempty" gorm:"type:varchar(128);default:''"`
	// Source is a stable, non-sensitive history category (for example
	// payment_method, promo_code, or referral_income). It intentionally never
	// contains user IDs, order IDs, or usernames.
	Source string `json:"source,omitempty" gorm:"type:varchar(32);default:''"`
	// Immutable payment quote captured when the order is created. These fields
	// let callbacks settle the original amount even after rates/settings change.
	PaymentCurrency      string  `json:"payment_currency" gorm:"type:varchar(8);not null;default:'USD'"`
	PaymentRateToUSD     float64 `json:"payment_rate_to_usd" gorm:"not null;default:1"`
	PaymentCoefficient   float64 `json:"payment_coefficient" gorm:"not null;default:1"`
	PaymentBaseAmount    float64 `json:"payment_base_amount" gorm:"not null;default:0"`
	PaymentCommission    float64 `json:"payment_commission" gorm:"not null;default:0"`
	PaymentChargedAmount float64 `json:"payment_charged_amount" gorm:"not null;default:0"`
	QuotaToAdd           int     `json:"quota_to_add"`
	CreateTime           int64   `json:"create_time"`
	CompleteTime         int64   `json:"complete_time"`
	Status               string  `json:"status"`
	// AccountingAmountUSD is a presentation-only snapshot populated for
	// history responses. It is deliberately not persisted: payment settlement
	// keeps using the immutable payment fields above, while the wallet can
	// avoid interpreting provider-currency amounts as USD.
	AccountingAmountUSD float64 `json:"accounting_amount_usd" gorm:"-"`
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
	ErrPaymentMethodMismatch    = errors.New("payment method mismatch")
	ErrTopUpNotFound            = errors.New("topup not found")
	ErrTopUpStatusInvalid       = errors.New("topup status invalid")
	ErrTopUpExpired             = errors.New("topup expired")
	ErrTopUpUserNotFound        = errors.New("topup user not found")
	ErrTopUpSettlementAmbiguous = errors.New("topup settlement amount is ambiguous")
)

// creditReferralDepositReward credits a percentage of a successful top-up to
// the direct inviter only. It runs inside the same completion transaction as
// the user's balance update, so the winning status CAS makes retries
// idempotent and no inviter chain is traversed.
func creditReferralDepositReward(tx *gorm.DB, topUp *TopUp, quotaToAdd int) error {
	percent := common.GetReferralDepositPercent()
	if percent <= 0 || percent > 100 || math.IsNaN(percent) || math.IsInf(percent, 0) || quotaToAdd <= 0 {
		return nil
	}

	reward := decimal.NewFromInt(int64(quotaToAdd)).
		Mul(decimal.NewFromFloat(percent)).
		Div(decimal.NewFromInt(100)).IntPart()
	if reward <= 0 {
		return nil
	}

	var referredUser User
	if err := tx.Select("inviter_id").First(&referredUser, topUp.UserId).Error; err != nil {
		return err
	}
	if referredUser.InviterId == 0 || referredUser.InviterId == topUp.UserId {
		return nil
	}
	// Only the immediate inviter is credited. Do not recursively credit the
	// inviter's inviter, even when the referred user has an inviter chain.
	result := tx.Model(&User{}).
		Where("id = ?", referredUser.InviterId).
		Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota + ?", reward),
			"aff_history": gorm.Expr("aff_history + ?", reward),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// A deleted inviter does not receive a reward or a dangling history row.
		return nil
	}

	// Keep referral income in the same paginated top-up history as paid
	// deposits. The source is deliberately categorical; no IDs or usernames
	// are exposed to the wallet UI.
	requestedAmount := 0.0
	quotaPerUnit := common.GetQuotaPerUnit()
	if common.IsValidQuotaPerUnitValue(quotaPerUnit) {
		requestedAmount = decimal.NewFromInt(reward).Div(decimal.NewFromFloat(quotaPerUnit)).InexactFloat64()
	}
	return tx.Create(&TopUp{
		UserId:            referredUser.InviterId,
		Amount:            reward,
		RequestedAmount:   requestedAmount,
		TradeNo:           fmt.Sprintf("REFERRAL-%s", common.GetRandomString(16)),
		PaymentMethod:     "referral",
		PaymentProvider:   "affiliate",
		PaymentCurrency:   "USD",
		PaymentBaseAmount: quotaToUSD(reward),
		Source:            "referral_income",
		QuotaToAdd:        int(reward),
		CreateTime:        common.GetTimestamp(),
		CompleteTime:      common.GetTimestamp(),
		Status:            common.TopUpStatusSuccess,
	}).Error
}

// invalidateTopUpUserCaches runs only after a completion transaction commits.
// Top-up completion updates quota with SQL expressions, so the referred user's
// cached wallet value must be discarded before their next read. Referral
// balances are kept in AffQuota/AffHistoryQuota and are not part of UserBase.
func invalidateTopUpUserCaches(topUp *TopUp) {
	if !common.RedisEnabled {
		return
	}
	if err := invalidateUserCache(topUp.UserId); err != nil {
		common.SysLog("failed to invalidate top-up user cache: " + err.Error())
	}
}

func (topUp *TopUp) Insert() error {
	if topUp != nil && topUp.UserId != 0 {
		_ = ExpireStalePendingTopUps(topUp.UserId)
	}
	var err error
	err = DB.Create(topUp).Error
	return err
}

// ExpireStalePendingTopUps closes stale local orders before they are shown or
// another order is created. Settlement paths perform the same check while
// holding the order lock, so a late provider callback cannot credit an expired
// order.
func ExpireStalePendingTopUps(userID int) error {
	query := DB.Where("status = ? AND create_time > 0", common.TopUpStatusPending)
	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}
	var pending []TopUp
	if err := query.Find(&pending).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for i := range pending {
		topUp := &pending[i]
		if now-topUp.CreateTime < int64(operation_setting.PendingTopUpTTL(topUp.PaymentMethod)/time.Second) {
			continue
		}
		result := DB.Model(&TopUp{}).
			Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).
			Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now})
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

// PaymentMethodDisplayName returns the safe, user-facing name for a payment
// method. It is used while an order is created so history preserves the name
// the user actually saw even if an administrator later edits PayMethods.
func PaymentMethodDisplayName(paymentMethod string) string {
	method := strings.TrimSpace(paymentMethod)
	if method == "" {
		return ""
	}
	for _, configured := range operation_setting.PayMethodsSnapshot() {
		if !strings.EqualFold(strings.TrimSpace(configured["type"]), method) {
			continue
		}
		name := strings.TrimSpace(configured["name"])
		if validStoredPaymentMethodName(method, name) {
			if strings.EqualFold(method, PaymentMethodYooKassaSBP) && (strings.EqualFold(name, "СБП / YooKassa") || strings.EqualFold(name, "YooKassa SBP")) {
				return "СБП"
			}
			return name
		}
		break
	}
	if name, ok := canonicalPaymentMethodDisplayName(method); ok {
		return name
	}
	if strings.EqualFold(method, PaymentMethodWaffo) {
		return "Waffo (Global Payment)"
	}
	return ""
}

func canonicalPaymentMethodDisplayName(paymentMethod string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(paymentMethod)) {
	case "alipay":
		return "Alipay", true
	case "wxpay":
		return "WeChat Pay", true
	case PaymentMethodStripe:
		return "Stripe", true
	case PaymentMethodCreem:
		return "Creem", true
	case PaymentMethodWaffoPancake:
		return "Waffo Pancake", true
	case PaymentMethodYooKassaSBP:
		return "СБП", true
	case PaymentMethodNOWPayments:
		return "Crypto / NOWPayments", true
	case PaymentMethodBalance:
		return "Balance top-up", true
	}
	return "", false
}

func validStoredPaymentMethodName(paymentMethod, stored string) bool {
	stored = strings.TrimSpace(stored)
	if stored == "" || len(stored) > 128 {
		return false
	}
	for _, char := range stored {
		if unicode.IsControl(char) {
			return false
		}
	}
	// Names are captured only after server-side validation at checkout creation.
	// Do not consult current PayMethods here: an administrator may rename or
	// delete a method after the payment was opened.
	return true
}

func annotateTopupSources(topups []*TopUp) {
	for _, topUp := range topups {
		if topUp == nil {
			continue
		}
		topUp.AccountingAmountUSD = topUpAccountingAmountUSD(topUp)
		switch strings.ToLower(strings.TrimSpace(topUp.Source)) {
		case "promo", "promo_code", "redemption", "redemption_code",
			"referral", "referral_income", "referral_reward", "affiliate":
			// Stable, non-sensitive categories are safe to expose.
		default:
			topUp.Source = ""
			if strings.EqualFold(strings.TrimSpace(topUp.PaymentMethod), PaymentMethodYooKassaSBP) && strings.EqualFold(strings.TrimSpace(topUp.PaymentMethodName), "yookassa") {
				topUp.PaymentMethodName = "СБП"
			} else if !validStoredPaymentMethodName(topUp.PaymentMethod, topUp.PaymentMethodName) {
				topUp.PaymentMethodName = PaymentMethodDisplayName(topUp.PaymentMethod)
			}
		}
	}
}

func topUpAccountingAmountUSD(topUp *TopUp) float64 {
	if topUp == nil {
		return 0
	}
	// New payment orders capture the wallet amount in USD independently of
	// the provider's settlement currency. This is the authoritative value for
	// history and remains valid after exchange-rate changes.
	if isFinitePositiveTopUpAmount(topUp.PaymentBaseAmount) {
		return topUp.PaymentBaseAmount
	}
	// Legacy records without a settlement snapshot do not contain enough
	// information to reconstruct the historical USD amount after QuotaPerUnit
	// changes. Never derive a value from the current setting: for known
	// provider currencies the UI can use the captured provider amount, while
	// USD legacy rows can safely use their original requested/money amount.
	if !legacyTopUpCanUseUSDAmount(topUp) {
		return 0
	}
	values := []float64{topUp.RequestedAmount, topUp.Money}
	switch strings.ToLower(strings.TrimSpace(topUp.PaymentProvider)) {
	case PaymentProviderStripe, PaymentProviderEpay, PaymentProviderWaffoPancake, PaymentProviderBalance:
		// These providers settle in USD. For legacy token-display rows the
		// requested amount may be raw tokens, while Money is the provider USD
		// amount and is therefore the safer historical accounting fallback.
		values = []float64{topUp.Money, topUp.RequestedAmount}
	}
	for _, value := range values {
		if isFinitePositiveTopUpAmount(value) {
			return value
		}
	}
	return 0
}

// legacyTopUpCanUseUSDAmount reports whether the legacy USD fields can be
// trusted when no immutable accounting snapshot exists. PaymentCurrency has a
// migration default of USD, so provider contracts must take precedence: a
// NOWPayments row, for example, historically stored USDT/token values while
// still carrying the default USD currency.
func legacyTopUpCanUseUSDAmount(topUp *TopUp) bool {
	if topUp == nil {
		return false
	}
	if currency := strings.ToUpper(strings.TrimSpace(topUp.PaymentCurrency)); currency != "" && currency != "USD" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(topUp.PaymentProvider)) {
	case PaymentProviderNOWPayments, PaymentProviderYooKassa, PaymentProviderWaffo, PaymentProviderCreem:
		return false
	default:
		return true
	}
}

// quotaToUSD captures the wallet accounting value when a quota-only history
// row is created. The value is stored on the row so later changes to
// QuotaPerUnit cannot rewrite the historical amount.
func quotaToUSD(quota int64) float64 {
	quotaPerUnit := common.GetQuotaPerUnit()
	if quota <= 0 || quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return 0
	}
	return decimal.NewFromInt(quota).Div(decimal.NewFromFloat(quotaPerUnit)).InexactFloat64()
}

func isFinitePositiveTopUpAmount(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
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
	topUp, err := GetTopUpByTradeNoWithError(tradeNo)
	if err != nil {
		return nil
	}
	return topUp
}

// GetTopUpByTradeNoWithError preserves database failures for webhook
// handlers. The pointer-only helper above remains a best-effort read API.
func GetTopUpByTradeNoWithError(tradeNo string) (*TopUp, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, errors.New("tradeNo is empty")
	}
	var topUp TopUp
	if err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	return &topUp, nil
}

// IsPermanentTopUpError reports settlement failures that a provider callback
// must acknowledge instead of retrying. A missing user is terminal only for
// orders with an immutable payment snapshot; legacy rows remain retryable
// because their historical settlement amount may be ambiguous.
func IsPermanentTopUpError(err error, topUp *TopUp) bool {
	if errors.Is(err, ErrTopUpStatusInvalid) || errors.Is(err, ErrTopUpExpired) || errors.Is(err, ErrPaymentMethodMismatch) || errors.Is(err, ErrTopUpSettlementAmbiguous) {
		return true
	}
	return errors.Is(err, ErrTopUpUserNotFound) && hasPaymentSettlementSnapshot(topUp)
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	expired := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if topUp.CreateTime > 0 && common.GetTimestamp()-topUp.CreateTime >= int64(operation_setting.PendingTopUpTTL(topUp.PaymentMethod)/time.Second) {
			topUp.Status = common.TopUpStatusExpired
			topUp.CompleteTime = common.GetTimestamp()
			if err := tx.Save(topUp).Error; err != nil {
				return err
			}
			// The callback is terminal from the provider's perspective; return
			// after committing the expired state below.
			expired = true
			return nil
		}

		topUp.Status = targetStatus
		topUp.CompleteTime = common.GetTimestamp()
		return tx.Save(topUp).Error
	})
	if err != nil {
		return err
	}
	if expired {
		return ErrTopUpStatusInvalid
	}
	return nil
}

// FailTopUpOrder closes a pending order when a provider has returned a
// terminal rejection or the paid user's account was permanently deleted.
// Ambiguous transport failures must not call this function.
func FailTopUpOrder(tradeNo string, expectedPaymentProvider string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		query := tx.Where(refCol+" = ?", tradeNo)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Set("gorm:query_option", "FOR UPDATE")
		}
		if err := query.First(&topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return nil
		}
		topUp.Status = common.TopUpStatusFailed
		topUp.CompleteTime = common.GetTimestamp()
		return tx.Save(&topUp).Error
	})
}

type topUpCompletionPrepare func(*gorm.DB, *TopUp) (map[string]interface{}, error)
type topUpCompletionApply func(*gorm.DB, *TopUp) error

func completeTopUpCAS(tradeNo, expectedProvider string, prepare topUpCompletionPrepare, apply topUpCompletionApply) (*TopUp, bool, error) {
	var topUp TopUp
	casLost := false
	alreadyCompleted := false
	expired := false
	completeTime := common.GetTimestamp()
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = DB.Transaction(func(tx *gorm.DB) error {
			query := tx.Where("trade_no = ?", tradeNo)
			if tx.Dialector.Name() != "sqlite" {
				query = query.Set("gorm:query_option", "FOR UPDATE")
			}
			if err := query.First(&topUp).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTopUpNotFound
				}
				return err
			}
			if expectedProvider != "" && topUp.PaymentProvider != expectedProvider {
				return ErrPaymentMethodMismatch
			}
			if topUp.Status == common.TopUpStatusSuccess {
				alreadyCompleted = true
				return nil
			}
			if topUp.Status != common.TopUpStatusPending {
				return ErrTopUpStatusInvalid
			}
			if topUp.CreateTime > 0 && common.GetTimestamp()-topUp.CreateTime >= int64(operation_setting.PendingTopUpTTL(topUp.PaymentMethod)/time.Second) {
				topUp.Status = common.TopUpStatusExpired
				topUp.CompleteTime = completeTime
				if err := tx.Save(&topUp).Error; err != nil {
					return err
				}
				expired = true
				return nil
			}

			updates, err := prepare(tx, &topUp)
			if err != nil {
				return err
			}
			if updates == nil {
				updates = map[string]interface{}{}
			}
			updates["complete_time"] = completeTime
			updates["status"] = common.TopUpStatusSuccess
			result := tx.Model(&TopUp{}).
				Where("id = ? AND payment_provider = ? AND status = ?", topUp.Id, topUp.PaymentProvider, common.TopUpStatusPending).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				casLost = true
				return nil
			}
			return apply(tx, &topUp)
		})
		if err == nil || !isSQLiteBusyError(err) {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	if err != nil {
		if isSQLiteBusyError(err) {
			current := &TopUp{}
			if lookupErr := DB.Where("trade_no = ?", tradeNo).First(current).Error; lookupErr == nil && current.Status == common.TopUpStatusSuccess {
				return current, false, nil
			}
		}
		// A verified payment for a hard-deleted user cannot ever be credited.
		// Mark snapshot-backed orders terminal for manual recovery; legacy rows
		// remain pending because their historical settlement is ambiguous.
		if errors.Is(err, ErrTopUpUserNotFound) && hasPaymentSettlementSnapshot(&topUp) {
			_ = FailTopUpOrder(tradeNo, expectedProvider)
		}
		if errors.Is(err, ErrTopUpSettlementAmbiguous) {
			// Legacy rows do not contain the historical conversion factor. Mark
			// them failed so a provider cannot retry and eventually over-credit
			// the account after QuotaPerUnit changes. Reconciliation is manual.
			_ = FailTopUpOrder(tradeNo, expectedProvider)
		}
		return nil, false, err
	}
	if expired {
		return nil, false, ErrTopUpExpired
	}
	if casLost {
		current := &TopUp{}
		if err := DB.Where("id = ?", topUp.Id).First(current).Error; err != nil {
			return nil, false, err
		}
		if current.PaymentProvider != topUp.PaymentProvider {
			return nil, false, ErrPaymentMethodMismatch
		}
		if current.Status == common.TopUpStatusSuccess {
			return current, false, nil
		}
		return nil, false, ErrTopUpStatusInvalid
	}
	if alreadyCompleted {
		return &topUp, false, nil
	}

	topUp.CompleteTime = completeTime
	topUp.Status = common.TopUpStatusSuccess
	invalidateTopUpUserCaches(&topUp)
	return &topUp, true, nil
}

func isSQLiteBusyError(err error) bool {
	return DB != nil && DB.Dialector.Name() == "sqlite" && strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

func resolveTopUpQuota(topUp *TopUp) (int, error) {
	return resolveTopUpQuotaWithDB(DB, topUp)
}

func resolveTopUpQuotaWithDB(db *gorm.DB, topUp *TopUp) (int, error) {
	if topUp.QuotaToAdd > 0 {
		return topUp.QuotaToAdd, nil
	}
	// A newly-created immutable payment snapshot may legitimately settle to
	// zero when commission consumes the entire gross amount. Do not treat that
	// valid result as a missing quota and fall back to legacy Amount formulas.
	if hasPaymentSettlementSnapshot(topUp) {
		return 0, nil
	}

	switch topUp.PaymentProvider {
	case PaymentProviderCreem:
		if topUp.Amount > 0 {
			return int(topUp.Amount), nil
		}
	case PaymentProviderStripe:
		return 0, ErrTopUpSettlementAmbiguous
	case PaymentProviderYooKassa:
		paymentMetadata := getPaymentMetadataByTradeNo(db, topUp.TradeNo)
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
	case PaymentProviderEpay, PaymentProviderWaffo, PaymentProviderWaffoPancake:
		return 0, ErrTopUpSettlementAmbiguous
	default:
		quota := int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.GetQuotaPerUnit())).IntPart())
		if quota > 0 {
			return quota, nil
		}
	}

	return 0, errors.New("无效或不明确的充值额度")
}

func hasPaymentSettlementSnapshot(topUp *TopUp) bool {
	return topUp != nil && topUp.RequestedAmount > 0 && topUp.PaymentBaseAmount > 0 && topUp.PaymentChargedAmount > 0
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	topUp, completed, err := completeTopUpCAS(referenceId, PaymentProviderStripe, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuotaWithDB(tx, topUp)
		quota = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quota)
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
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
	topUp, completed, err := completeTopUpCAS(tradeNo, PaymentProviderEpay, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		var resolveErr error
		quotaToAdd, resolveErr = resolveTopUpQuotaWithDB(tx, topUp)
		if resolveErr != nil {
			return nil, resolveErr
		}
		updates := map[string]interface{}{}
		if paymentMethod != "" {
			updates["payment_method"] = paymentMethod
		}
		return updates, nil
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quotaToAdd)
	})
	if err != nil {
		if errors.Is(err, ErrTopUpUserNotFound) && hasPaymentSettlementSnapshot(topUp) {
			_ = FailTopUpOrder(tradeNo, PaymentProviderEpay)
		}
		common.SysError("epay topup failed: " + err.Error())
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
	}
	if completed {
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
	if err = ExpireStalePendingTopUps(userId); err != nil {
		return nil, 0, err
	}
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
	annotateTopupSources(topups)

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	if err = ExpireStalePendingTopUps(0); err != nil {
		return nil, 0, err
	}
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
	annotateTopupSources(topups)

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
	if err = ExpireStalePendingTopUps(userId); err != nil {
		return nil, 0, err
	}
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
	annotateTopupSources(topups)

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	if err = ExpireStalePendingTopUps(0); err != nil {
		return nil, 0, err
	}
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
	annotateTopupSources(topups)

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
	topUp, completed, err := completeTopUpCAS(tradeNo, "", func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuotaWithDB(tx, topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quotaToAdd)
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
	topUp, completed, err := completeTopUpCAS(referenceId, PaymentProviderCreem, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, err := resolveTopUpQuotaWithDB(tx, topUp)
		quota = int64(resolvedQuota)
		return nil, err
	}, func(tx *gorm.DB, topUp *TopUp) error {
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}
		if customerEmail != "" {
			var user User
			if err := tx.Where("id = ?", topUp.UserId).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTopUpUserNotFound
				}
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
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, int(quota))
	})
	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
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
	topUp, completed, err := completeTopUpCAS(tradeNo, PaymentProviderWaffo, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuotaWithDB(tx, topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quotaToAdd)
	})
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
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
	topUp, completed, err := completeTopUpCAS(tradeNo, PaymentProviderWaffoPancake, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuotaWithDB(tx, topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quotaToAdd)
	})
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
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
	topUp, completed, err := completeTopUpCAS(tradeNo, provider, func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
		resolvedQuota, resolveErr := resolveTopUpQuotaWithDB(tx, topUp)
		quotaToAdd = resolvedQuota
		return nil, resolveErr
	}, func(tx *gorm.DB, topUp *TopUp) error {
		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTopUpUserNotFound
		}
		return creditReferralDepositReward(tx, topUp, quotaToAdd)
	})
	if err != nil {
		common.SysError(strings.ToLower(providerName) + " topup failed: " + err.Error())
		return fmt.Errorf("Top-up failed, please try again later: %w", err)
	}
	if completed {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("%s top-up succeeded, quota: %v, payment amount: %.2f", providerName, logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, provider)
	}

	return nil
}
