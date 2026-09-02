package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

const deprecatedReferralCashbackOption = "ReferralCashbackPercent"

// removeDeprecatedReferralCashbackOption removes the obsolete global referral
// cashback setting. Referral cashback is configured per amount tier instead.
func removeDeprecatedReferralCashbackOption() error {
	if err := DB.Where("key = ?", deprecatedReferralCashbackOption).Delete(&Option{}).Error; err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, deprecatedReferralCashbackOption)
	common.OptionMapRWMutex.Unlock()
	return nil
}

// GetPayMethodsFromDB reads the persisted payment-method catalog. A missing
// options table is treated as an uninitialized database so callers can use
// their bootstrap snapshot; other database failures are preserved and must
// not silently re-enable stale payment methods.
func GetPayMethodsFromDB(db *gorm.DB) ([]map[string]string, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var option Option
	if err := db.Where("key = ?", "PayMethods").First(&option).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingOptionsTableError(err) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	methods, err := operation_setting.ParsePayMethodsJSON(option.Value)
	if err != nil {
		return nil, err
	}
	return filterRetiredPayMethods(operation_setting.CanonicalizePayMethods(methods)), nil
}

func filterRetiredPayMethods(methods []map[string]string) []map[string]string {
	filtered := make([]map[string]string, 0, len(methods))
	for _, method := range methods {
		if method == nil || strings.EqualFold(strings.TrimSpace(method["type"]), PaymentMethodCreem) {
			continue
		}
		filtered = append(filtered, method)
	}
	return filtered
}

// removeRetiredCreemPayMethod removes the retired checkout from the durable
// catalog. Existing orders keep their own provider snapshot and can still be
// settled by the legacy webhook.
func removeRetiredCreemPayMethod() error {
	var option Option
	if err := DB.First(&option, "key = ?", "PayMethods").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	methods, err := operation_setting.ParsePayMethodsJSON(option.Value)
	if err != nil {
		return err
	}
	hasCreem := false
	for _, method := range methods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), PaymentMethodCreem) {
			hasCreem = true
			break
		}
	}
	if !hasCreem {
		return nil
	}
	encoded, err := common.Marshal(filterRetiredPayMethods(operation_setting.CanonicalizePayMethods(methods)))
	if err != nil {
		return err
	}
	result := DB.Model(&Option{}).Where("key = ? AND value = ?", "PayMethods", option.Value).
		Update("value", string(encoded))
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	return updateOptionMapFromDatabase("PayMethods", string(encoded))
}

// HasDirectUSDTMethod reports whether the catalog explicitly enables the
// direct USDT integration. Presence in persisted PayMethods, rather than the
// legacy integration flag, is the runtime activation switch.
func HasDirectUSDTMethod(methods []map[string]string) bool {
	for _, method := range methods {
		if method != nil && isDirectUSDTNetworkProvider(method["type"]) {
			return true
		}
	}
	return false
}

func isDirectUSDTNetworkProvider(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case operation_setting.DirectCryptoPaymentMethod, operation_setting.DirectUSDTTRC20PaymentMethod, operation_setting.DirectUSDTTONPaymentMethod, operation_setting.DirectUSDTSolanaPaymentMethod:
		return true
	default:
		return false
	}
}

// IsDirectUSDTMethodConfigured reads the persisted catalog. Databases created
// before the options table existed keep the old test/bootstrap behaviour and
// fall back to the legacy flag; once the table exists, a missing row or method
// is an explicit disabled state.
func IsDirectUSDTMethodConfigured() bool {
	methods, err := GetPayMethodsFromDB(DB)
	if err == nil {
		return HasDirectUSDTMethod(methods)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) || DB == nil {
		return false
	}
	var option Option
	lookupErr := DB.Where("key = ?", "PayMethods").First(&option).Error
	if isMissingOptionsTableError(lookupErr) {
		return setting.USDTTRC20Enabled
	}
	return false
}

func isMissingOptionsTableError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") ||
		strings.Contains(message, `relation "options" does not exist`) ||
		(strings.Contains(message, "table") && strings.Contains(message, "doesn't exist") && strings.Contains(message, "options"))
}

const removedChatsOptionKey = "Chats"

// JSONObjectPatch describes changes to one JSON-object option. Set values are
// applied after Delete, so Set wins when a field is present in both lists.
type JSONObjectPatch struct {
	Set    map[string]any `json:"set,omitempty"`
	Delete []string       `json:"delete,omitempty"`
}

var optionUpdateMutex sync.Mutex
var modelRequestRateLimitActivationNow = common.GetTimestamp

func modelRequestRateLimitActivationDelay() int64 {
	return int64(max(2*common.SyncFrequency, 2) + 5)
}

// LockPricingOptionSnapshot prevents a reader from observing a committed
// option version before the corresponding in-memory pricing maps are refreshed.
func LockPricingOptionSnapshot() func() {
	optionUpdateMutex.Lock()
	return optionUpdateMutex.Unlock
}

// jsonObjectPatchOptionKeys is intentionally limited to model-pricing maps.
// Unlike a regular option update, patching an arbitrary JSON option cannot
// safely run the option-specific migration/normalization pipeline while also
// merging the latest value under a row lock. Keep this API scoped to the
// pricing synchronizer's independent per-model maps.
var jsonObjectPatchOptionKeys = map[string]struct{}{
	"ModelRatio":                      {},
	"ModelPrice":                      {},
	"CompletionRatio":                 {},
	"CacheRatio":                      {},
	"CreateCacheRatio":                {},
	"ImageRatio":                      {},
	"AudioRatio":                      {},
	"AudioCompletionRatio":            {},
	"billing_setting.billing_mode":    {},
	"billing_setting.billing_expr":    {},
	"billing_setting.task_price_unit": {},
}

func IsModelPricingOption(key string) bool {
	_, ok := jsonObjectPatchOptionKeys[key]
	return ok
}

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	yooKassaConfig := setting.GetYooKassaConfig()

	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)

	// 添加原有的系统配置
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["PasswordLoginEnabled"] = strconv.FormatBool(common.PasswordLoginEnabled)
	common.OptionMap["PasswordRegisterEnabled"] = strconv.FormatBool(common.PasswordRegisterEnabled)
	common.OptionMap["EmailVerificationEnabled"] = strconv.FormatBool(common.EmailVerificationEnabled)
	common.OptionMap["GitHubOAuthEnabled"] = strconv.FormatBool(common.GitHubOAuthEnabled)
	common.OptionMap["LinuxDOOAuthEnabled"] = strconv.FormatBool(common.LinuxDOOAuthEnabled)
	common.OptionMap["TelegramOAuthEnabled"] = strconv.FormatBool(common.TelegramOAuthEnabled)
	common.OptionMap["WeChatAuthEnabled"] = strconv.FormatBool(common.WeChatAuthEnabled)
	common.OptionMap["TurnstileCheckEnabled"] = strconv.FormatBool(common.TurnstileCheckEnabled)
	common.OptionMap["RegisterEnabled"] = strconv.FormatBool(common.RegisterEnabled)
	common.OptionMap["AutomaticDisableChannelEnabled"] = strconv.FormatBool(common.AutomaticDisableChannelEnabled)
	common.OptionMap["AutomaticEnableChannelEnabled"] = strconv.FormatBool(common.AutomaticEnableChannelEnabled)
	common.OptionMap["LogConsumeEnabled"] = strconv.FormatBool(common.LogConsumeEnabled)
	common.OptionMap["DisplayInCurrencyEnabled"] = strconv.FormatBool(common.DisplayInCurrencyEnabled)
	common.OptionMap["DisplayTokenStatEnabled"] = strconv.FormatBool(common.DisplayTokenStatEnabled)
	common.OptionMap["DrawingEnabled"] = strconv.FormatBool(common.DrawingEnabled)
	common.OptionMap["TaskEnabled"] = strconv.FormatBool(common.TaskEnabled)
	common.OptionMap["DataExportEnabled"] = strconv.FormatBool(common.DataExportEnabled)
	common.OptionMap["ChannelDisableThreshold"] = strconv.FormatFloat(common.ChannelDisableThreshold, 'f', -1, 64)
	common.OptionMap["EmailDomainRestrictionEnabled"] = strconv.FormatBool(common.EmailDomainRestrictionEnabled)
	common.OptionMap["EmailAliasRestrictionEnabled"] = strconv.FormatBool(common.EmailAliasRestrictionEnabled)
	common.OptionMap["EmailDomainWhitelist"] = strings.Join(common.EmailDomainWhitelist, ",")
	common.OptionMap["SMTPServer"] = ""
	common.OptionMap["SMTPFrom"] = ""
	common.OptionMap["SMTPPort"] = strconv.Itoa(common.SMTPPort)
	common.OptionMap["SMTPAccount"] = ""
	common.OptionMap["SMTPToken"] = ""
	common.OptionMap["SMTPSSLEnabled"] = strconv.FormatBool(common.SMTPSSLEnabled)
	common.OptionMap["SMTPStartTLSEnabled"] = strconv.FormatBool(common.SMTPStartTLSEnabled)
	common.OptionMap["SMTPInsecureSkipVerify"] = strconv.FormatBool(common.SMTPInsecureSkipVerify)
	common.OptionMap["SMTPForceAuthLogin"] = strconv.FormatBool(common.SMTPForceAuthLogin)
	common.OptionMap["Notice"] = ""
	common.OptionMap["About"] = ""
	common.OptionMap["HomePageContent"] = ""
	common.OptionMap["Footer"] = common.Footer
	common.OptionMap["SystemName"] = common.SystemName
	common.OptionMap["Logo"] = common.Logo
	common.OptionMap["ServerAddress"] = ""
	common.OptionMap["WorkerUrl"] = system_setting.WorkerUrl
	common.OptionMap["WorkerValidKey"] = system_setting.WorkerValidKey
	common.OptionMap["WorkerAllowHttpImageRequestEnabled"] = strconv.FormatBool(system_setting.WorkerAllowHttpImageRequestEnabled)
	common.OptionMap["PayAddress"] = ""
	common.OptionMap["CustomCallbackAddress"] = ""
	common.OptionMap["EpayId"] = ""
	common.OptionMap["EpayKey"] = ""
	common.OptionMap["Price"] = strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	common.OptionMap["USDExchangeRate"] = strconv.FormatFloat(operation_setting.USDExchangeRate, 'f', -1, 64)
	common.OptionMap["MinTopUp"] = strconv.FormatFloat(operation_setting.MinTopUp, 'f', -1, 64)
	common.OptionMap["StripeMinTopUp"] = strconv.FormatFloat(setting.StripeMinTopUp, 'f', -1, 64)
	common.OptionMap["StripeApiSecret"] = setting.StripeApiSecret
	common.OptionMap["StripeWebhookSecret"] = setting.StripeWebhookSecret
	common.OptionMap["StripePriceId"] = setting.StripePriceId
	common.OptionMap["StripeUnitPrice"] = strconv.FormatFloat(setting.StripeUnitPrice, 'f', -1, 64)
	common.OptionMap["StripePromotionCodesEnabled"] = strconv.FormatBool(setting.StripePromotionCodesEnabled)
	common.OptionMap["CreemApiKey"] = setting.CreemApiKey
	common.OptionMap["CreemProducts"] = setting.CreemProducts
	common.OptionMap["CreemTestMode"] = strconv.FormatBool(setting.CreemTestMode)
	common.OptionMap["CreemWebhookSecret"] = setting.CreemWebhookSecret
	common.OptionMap["WaffoEnabled"] = strconv.FormatBool(setting.WaffoEnabled)
	common.OptionMap["WaffoApiKey"] = setting.WaffoApiKey
	common.OptionMap["WaffoPrivateKey"] = setting.WaffoPrivateKey
	common.OptionMap["WaffoPublicCert"] = setting.WaffoPublicCert
	common.OptionMap["WaffoSandboxPublicCert"] = setting.WaffoSandboxPublicCert
	common.OptionMap["WaffoSandboxApiKey"] = setting.WaffoSandboxApiKey
	common.OptionMap["WaffoSandboxPrivateKey"] = setting.WaffoSandboxPrivateKey
	common.OptionMap["WaffoSandbox"] = strconv.FormatBool(setting.WaffoSandbox)
	common.OptionMap["WaffoMerchantId"] = setting.WaffoMerchantId
	common.OptionMap["WaffoNotifyUrl"] = setting.WaffoNotifyUrl
	common.OptionMap["WaffoReturnUrl"] = setting.WaffoReturnUrl
	common.OptionMap["WaffoSubscriptionReturnUrl"] = setting.WaffoSubscriptionReturnUrl
	common.OptionMap["WaffoCurrency"] = setting.WaffoCurrency
	common.OptionMap["WaffoUnitPrice"] = strconv.FormatFloat(setting.WaffoUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoMinTopUp"] = strconv.FormatFloat(setting.WaffoMinTopUp, 'f', -1, 64)
	common.OptionMap["WaffoPayMethods"] = setting.WaffoPayMethods2JsonString()
	common.OptionMap["WaffoPancakeMerchantID"] = setting.WaffoPancakeMerchantID
	common.OptionMap["WaffoPancakePrivateKey"] = setting.WaffoPancakePrivateKey
	common.OptionMap["WaffoPancakeReturnURL"] = setting.WaffoPancakeReturnURL
	common.OptionMap["WaffoPancakeUnitPrice"] = strconv.FormatFloat(setting.WaffoPancakeUnitPrice, 'f', -1, 64)
	common.OptionMap["WaffoPancakeMinTopUp"] = strconv.FormatFloat(setting.WaffoPancakeMinTopUp, 'f', -1, 64)
	common.OptionMap["WaffoPancakeStoreID"] = setting.WaffoPancakeStoreID
	common.OptionMap["WaffoPancakeProductID"] = setting.WaffoPancakeProductID
	common.OptionMap["YooKassaEnabled"] = strconv.FormatBool(yooKassaConfig.Enabled)
	common.OptionMap["YooKassaShopID"] = yooKassaConfig.ShopID
	common.OptionMap["YooKassaSecretKey"] = yooKassaConfig.SecretKey
	common.OptionMap["YooKassaReturnURL"] = yooKassaConfig.ReturnURL
	common.OptionMap["YooKassaPaymentMethods"] = yooKassaConfig.PaymentMethods
	common.OptionMap["NOWPaymentsEnabled"] = strconv.FormatBool(setting.NOWPaymentsEnabled)
	common.OptionMap["NOWPaymentsAPIKey"] = setting.NOWPaymentsAPIKey
	common.OptionMap["NOWPaymentsIPNSecret"] = setting.NOWPaymentsIPNSecret
	common.OptionMap["NOWPaymentsPriceCurrency"] = setting.NOWPaymentsPriceCurrency
	common.OptionMap["NOWPaymentsPayCurrency"] = setting.NOWPaymentsPayCurrency
	common.OptionMap["NOWPaymentsIPNCallbackURL"] = setting.NOWPaymentsIPNCallbackURL
	common.OptionMap["USDTTRC20Enabled"] = strconv.FormatBool(setting.USDTTRC20Enabled)
	common.OptionMap["USDTTRC20ReceivingAddress"] = setting.USDTTRC20ReceivingAddress
	common.OptionMap["USDTTONReceivingAddress"] = setting.USDTTONReceivingAddress
	common.OptionMap["USDTSolanaReceivingAddress"] = setting.USDTSolanaReceivingAddress
	common.OptionMap["USDTTRC20AmountPrecision"] = strconv.Itoa(setting.USDTTRC20AmountPrecision)
	common.OptionMap["USDTTRC20AmountTailLimitUnits"] = strconv.Itoa(setting.USDTTRC20AmountTailLimitUnits)
	// Legacy suffix bounds remain visible to old clients during migration, but
	// are no longer read by the direct payment runtime.
	common.OptionMap["USDTTRC20AmountSuffixMinUnits"] = strconv.Itoa(setting.USDTTRC20AmountSuffixMinUnits)
	common.OptionMap["USDTTRC20AmountSuffixMaxUnits"] = strconv.Itoa(setting.USDTTRC20AmountSuffixMaxUnits)
	common.OptionMap["USDTTRC20APIKey"] = setting.USDTTRC20APIKey
	common.OptionMap["USDTTONAPIKey"] = setting.USDTTONAPIKey
	common.OptionMap["USDTTONAPIBaseURL"] = setting.USDTTONAPIBaseURL
	common.OptionMap["USDTSolanaRPCURL"] = setting.USDTSolanaRPCURL
	common.OptionMap["USDTSolanaAPIKey"] = setting.USDTSolanaAPIKey
	common.OptionMap["USDTSolanaReceivingTokenAccount"] = setting.USDTSolanaReceivingTokenAccount
	common.OptionMap["USDTTRC20MaxCreationsPerHour"] = strconv.Itoa(setting.USDTTRC20MaxCreationsPerHour)
	common.OptionMap["USDTTRC20PaymentURLBase"] = setting.USDTTRC20PaymentURLBase
	common.OptionMap["TopupGroupRatio"] = common.TopupGroupRatio2JSONString()
	common.OptionMap["PricingGroups"] = "[]"
	common.OptionMap["AutoGroups"] = setting.AutoGroups2JsonString()
	common.OptionMap["DefaultUseAutoGroup"] = strconv.FormatBool(setting.DefaultUseAutoGroup)
	common.OptionMap["PayMethods"] = operation_setting.PayMethods2JsonString()
	common.OptionMap[operation_setting.PaymentPendingTTLMinutes] = "1440"
	common.OptionMap[operation_setting.PaymentCreationRateLimit] = "5"
	common.OptionMap[operation_setting.PaymentCreationRateLimitDurationMinutes] = "1"
	common.OptionMap["GitHubClientId"] = ""
	common.OptionMap["GitHubClientSecret"] = ""
	common.OptionMap["TelegramBotToken"] = ""
	common.OptionMap["TelegramBotName"] = ""
	common.OptionMap["WeChatServerAddress"] = ""
	common.OptionMap["WeChatServerToken"] = ""
	common.OptionMap["WeChatAccountQRCodeImageURL"] = ""
	common.OptionMap["TurnstileSiteKey"] = ""
	common.OptionMap["TurnstileSecretKey"] = ""
	common.OptionMap["QuotaForNewUser"] = strconv.Itoa(common.QuotaForNewUser)
	common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)
	common.OptionMap["QuotaForInvitee"] = strconv.Itoa(common.QuotaForInvitee)
	common.OptionMap["ReferralDepositPercent"] = strconv.FormatFloat(common.GetReferralDepositPercent(), 'f', -1, 64)
	common.OptionMap["ReferralRequiredTopUpUSD"] = strconv.FormatFloat(common.GetReferralRequiredTopUpUSD(), 'f', -1, 64)
	common.OptionMap["QuotaRemindThreshold"] = strconv.Itoa(common.QuotaRemindThreshold)
	common.OptionMap["PreConsumedQuota"] = strconv.Itoa(common.PreConsumedQuota)
	common.OptionMap["ModelRequestRateLimitCount"] = strconv.Itoa(setting.ModelRequestRateLimitCount)
	common.OptionMap["ModelRequestRateLimitDurationMinutes"] = strconv.Itoa(setting.ModelRequestRateLimitDurationMinutes)
	common.OptionMap[setting.ModelRequestRateLimitDurationOption] = setting.ModelRequestRateLimitDurationValue()
	common.OptionMap[setting.ModelRequestRateLimitDurationActivatedOption] = "false"
	common.OptionMap[setting.ModelRequestRateLimitDurationActivationAtOption] = "0"
	common.OptionMap[setting.ModelRequestRateLimitDurationActiveOption] = "false"
	common.OptionMap[setting.ModelRequestRateLimitDurationStagedOption] = "false"
	common.OptionMap["ModelRequestRateLimitSuccessCount"] = strconv.Itoa(setting.ModelRequestRateLimitSuccessCount)
	common.OptionMap["ModelRequestRateLimitGroup"] = setting.ModelRequestRateLimitGroup2JSONString()
	common.OptionMap["ModelRatio"] = ratio_setting.ModelRatio2JSONString()
	common.OptionMap["ModelPrice"] = ratio_setting.ModelPrice2JSONString()
	common.OptionMap["CacheRatio"] = ratio_setting.CacheRatio2JSONString()
	common.OptionMap["CreateCacheRatio"] = ratio_setting.CreateCacheRatio2JSONString()
	common.OptionMap["GroupRatio"] = "{}"
	common.OptionMap["GroupGroupRatio"] = ratio_setting.GroupGroupRatio2JSONString()
	common.OptionMap["CompletionRatio"] = ratio_setting.CompletionRatio2JSONString()
	common.OptionMap["ImageRatio"] = ratio_setting.ImageRatio2JSONString()
	common.OptionMap["AudioRatio"] = ratio_setting.AudioRatio2JSONString()
	common.OptionMap["AudioCompletionRatio"] = ratio_setting.AudioCompletionRatio2JSONString()
	common.OptionMap["PricingSyncStrategy"] = PricingSyncStrategyHighest
	common.OptionMap["TopUpLink"] = common.TopUpLink
	//common.OptionMap["ChatLink"] = common.ChatLink
	//common.OptionMap["ChatLink2"] = common.ChatLink2
	common.OptionMap["QuotaPerUnit"] = strconv.FormatFloat(common.GetQuotaPerUnit(), 'f', -1, 64)
	common.OptionMap["RetryTimes"] = strconv.Itoa(common.RetryTimes)
	common.OptionMap["DataExportInterval"] = strconv.Itoa(common.DataExportInterval)
	common.OptionMap["DataExportDefaultTime"] = common.DataExportDefaultTime
	common.OptionMap["DefaultCollapseSidebar"] = strconv.FormatBool(common.DefaultCollapseSidebar)
	common.OptionMap["MjNotifyEnabled"] = strconv.FormatBool(setting.MjNotifyEnabled)
	common.OptionMap["MjAccountFilterEnabled"] = strconv.FormatBool(setting.MjAccountFilterEnabled)
	common.OptionMap["MjModeClearEnabled"] = strconv.FormatBool(setting.MjModeClearEnabled)
	common.OptionMap["MjForwardUrlEnabled"] = strconv.FormatBool(setting.MjForwardUrlEnabled)
	common.OptionMap["MjActionCheckSuccessEnabled"] = strconv.FormatBool(setting.MjActionCheckSuccessEnabled)
	common.OptionMap["CheckSensitiveEnabled"] = strconv.FormatBool(setting.CheckSensitiveEnabled)
	common.OptionMap["DemoSiteEnabled"] = strconv.FormatBool(operation_setting.DemoSiteEnabled)
	common.OptionMap["SelfUseModeEnabled"] = strconv.FormatBool(operation_setting.SelfUseModeEnabled)
	common.OptionMap["ModelRequestRateLimitEnabled"] = strconv.FormatBool(setting.ModelRequestRateLimitEnabled)
	common.OptionMap["CheckSensitiveOnPromptEnabled"] = strconv.FormatBool(setting.CheckSensitiveOnPromptEnabled)
	common.OptionMap["StopOnSensitiveEnabled"] = strconv.FormatBool(setting.StopOnSensitiveEnabled)
	common.OptionMap["SensitiveWords"] = setting.SensitiveWordsToString()
	common.OptionMap["StreamCacheQueueLength"] = strconv.Itoa(setting.StreamCacheQueueLength)
	common.OptionMap["AutomaticDisableKeywords"] = operation_setting.AutomaticDisableKeywordsToString()
	common.OptionMap["AutomaticDisableStatusCodes"] = operation_setting.AutomaticDisableStatusCodesToString()
	common.OptionMap["AutomaticRetryStatusCodes"] = operation_setting.AutomaticRetryStatusCodesToString()
	common.OptionMap["ExposeRatioEnabled"] = strconv.FormatBool(ratio_setting.IsExposeRatioEnabled())

	// 自动添加所有注册的模型配置
	modelConfigs := config.GlobalConfig.ExportAllConfigs()
	for k, v := range modelConfigs {
		common.OptionMap[k] = v
	}

	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabaseLocked()
}

func loadOptionsFromDatabase() {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()

	loadOptionsFromDatabaseLocked()
}

func loadOptionsFromDatabaseLocked() {
	options, err := AllOption()
	if err != nil {
		common.SysLog("failed to load options: " + err.Error())
		return
	}
	hasPricingGroups := false
	hasLegacyUsableGroups := false
	hasModelRequestRateLimitDuration := false
	modelRequestRateLimitDurationActivated := false
	modelRequestRateLimitDurationActivationAt := int64(0)
	legacyUsableGroupsValid := true
	legacyGroupRatio := ""
	modelRequestRateLimitDuration := ""
	modelRequestRateLimitDurationMinutes := ""
	legacyUsableGroups := make(map[string]string)
	hasTailLimit := false
	legacyPrecision := 0
	for _, option := range options {
		switch option.Key {
		case "USDTTRC20AmountTailLimitUnits":
			hasTailLimit = true
		case "USDTTRC20AmountPrecision":
			legacyPrecision, _ = strconv.Atoi(strings.TrimSpace(option.Value))
		case "PricingGroups":
			hasPricingGroups = true
		case "GroupRatio":
			legacyGroupRatio = option.Value
		case "UserUsableGroups":
			hasLegacyUsableGroups = true
			if err := common.Unmarshal([]byte(option.Value), &legacyUsableGroups); err != nil {
				common.SysLog("failed to read legacy user usable groups: " + err.Error())
				legacyUsableGroupsValid = false
			}
		case setting.ModelRequestRateLimitDurationOption:
			hasModelRequestRateLimitDuration = true
			modelRequestRateLimitDuration = option.Value
		case setting.ModelRequestRateLimitDurationLegacyOption:
			modelRequestRateLimitDurationMinutes = option.Value
		case setting.ModelRequestRateLimitDurationActivatedOption:
			modelRequestRateLimitDurationActivated = option.Value == "true"
		case setting.ModelRequestRateLimitDurationActivationAtOption:
			modelRequestRateLimitDurationActivationAt, _ = strconv.ParseInt(option.Value, 10, 64)
		}
	}
	if !hasTailLimit {
		limit := setting.DefaultUSDTTRC20AmountTailLimitUnits
		if migrated, err := setting.USDTTRC20AmountTailLimitForPrecision(legacyPrecision); err == nil {
			limit = migrated
		}
		persistedLimit, err := ensureDirectUSDTAmountTailLimitPersisted(limit)
		if err != nil {
			common.SysLog("failed to migrate USDT TRC20 amount tail limit: " + err.Error())
		} else {
			limit = persistedLimit
			setting.USDTTRC20AmountTailLimitUnits = persistedLimit
			common.OptionMapRWMutex.Lock()
			common.OptionMap["USDTTRC20AmountTailLimitUnits"] = strconv.Itoa(limit)
			common.OptionMapRWMutex.Unlock()
		}
	}
	if !hasLegacyUsableGroups {
		legacyUsableGroups = map[string]string{
			"default": "默认分组",
			"vip":     "vip分组",
		}
	}
	sort.SliceStable(options, func(i, j int) bool {
		return optionLoadPriority(options[i].Key) < optionLoadPriority(options[j].Key)
	})
	for _, option := range options {
		if option.Key == removedChatsOptionKey || option.Key == operation_setting.DirectUSDTTRC20PayMethodsMigratedOption || option.Key == referralEligibilityBackfillOption || option.Key == deprecatedReferralCashbackOption {
			continue
		}
		if option.Key == "GroupRatio" {
			continue
		}
		err := updateOptionMapFromDatabase(option.Key, option.Value)
		if err != nil {
			common.SysLog("failed to update option map: " + err.Error())
			if option.Key == "PricingGroups" {
				return
			}
		}
	}
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey: setting.CreemApiKey, Products: setting.CreemProducts,
		TestMode: setting.CreemTestMode, WebhookSecret: setting.CreemWebhookSecret,
	})
	publishYooKassaConfig()
	if err := ensureYooKassaPayMethodPersisted(); err != nil {
		common.SysLog("failed to migrate YooKassa payment method: " + err.Error())
	}
	if err := ensureNOWPaymentsPayMethodPersisted(); err != nil {
		common.SysLog("failed to migrate NOWPayments payment method: " + err.Error())
	}
	if err := removeRetiredCreemPayMethod(); err != nil {
		common.SysLog("failed to remove retired Creem payment method: " + err.Error())
	}
	if err := removeDeprecatedReferralCashbackOption(); err != nil {
		common.SysLog("failed to remove deprecated referral cashback option: " + err.Error())
	}
	resolvedRateLimitDuration, err := applyModelRequestRateLimitDuration(
		modelRequestRateLimitDuration,
		hasModelRequestRateLimitDuration,
		modelRequestRateLimitDurationMinutes,
		modelRequestRateLimitDurationActivated,
		modelRequestRateLimitDurationActivationAt,
	)
	if err != nil {
		common.SysLog("failed to set model request rate limit duration: " + err.Error())
	}
	common.OptionMapRWMutex.Lock()
	if !hasModelRequestRateLimitDuration {
		common.OptionMap[setting.ModelRequestRateLimitDurationOption] = resolvedRateLimitDuration
	}
	common.OptionMap[setting.ModelRequestRateLimitDurationActivatedOption] = strconv.FormatBool(modelRequestRateLimitDurationActivated)
	common.OptionMap[setting.ModelRequestRateLimitDurationActivationAtOption] = strconv.FormatInt(modelRequestRateLimitDurationActivationAt, 10)
	common.OptionMap[setting.ModelRequestRateLimitDurationActiveOption] = strconv.FormatBool(setting.ModelRequestRateLimitDurationConfig().Canonical)
	common.OptionMap[setting.ModelRequestRateLimitDurationStagedOption] = strconv.FormatBool(hasModelRequestRateLimitDuration)
	common.OptionMapRWMutex.Unlock()
	if !hasPricingGroups && !legacyUsableGroupsValid {
		return
	}
	if !hasPricingGroups && legacyUsableGroupsValid {
		completed, err := migratePricingGroupsFromLegacy(legacyGroupRatio, legacyUsableGroups)
		if err != nil {
			common.SysLog("failed to migrate pricing groups: " + err.Error())
		}
		if !completed {
			return
		}
	}
	normalizePricingGroupOptionMaps()
	if err := NormalizePricingGroupReferences(); err != nil {
		common.SysLog("failed to normalize pricing group references: " + err.Error())
	}
}

func applyModelRequestRateLimitDuration(durationValue string, durationExists bool, legacyMinutes string, activated bool, activationAt int64) (string, error) {
	legacyResolved := setting.ResolveModelRequestRateLimitDuration("", false, legacyMinutes)
	canonicalResolved := legacyResolved
	if durationExists {
		canonicalResolved = setting.ResolveModelRequestRateLimitDuration(durationValue, true, legacyMinutes)
	}
	if err := setting.ConfigureModelRequestRateLimitDuration(canonicalResolved, legacyResolved, durationExists && activated, activationAt); err != nil {
		return "", err
	}
	return setting.ModelRequestRateLimitDurationValue(), nil
}

func refreshModelRequestRateLimitDuration() error {
	keys := []string{
		setting.ModelRequestRateLimitDurationOption,
		setting.ModelRequestRateLimitDurationLegacyOption,
		setting.ModelRequestRateLimitDurationActivatedOption,
		setting.ModelRequestRateLimitDurationActivationAtOption,
	}
	var options []Option
	if err := DB.Where("key IN ?", keys).Find(&options).Error; err != nil {
		return err
	}

	durationValue := ""
	legacyMinutes := ""
	durationExists := false
	activated := false
	activationAt := int64(0)
	for _, option := range options {
		switch option.Key {
		case setting.ModelRequestRateLimitDurationOption:
			durationValue = option.Value
			durationExists = true
		case setting.ModelRequestRateLimitDurationLegacyOption:
			legacyMinutes = option.Value
		case setting.ModelRequestRateLimitDurationActivatedOption:
			activated = option.Value == "true"
		case setting.ModelRequestRateLimitDurationActivationAtOption:
			activationAt, _ = strconv.ParseInt(option.Value, 10, 64)
		}
	}
	resolved, err := applyModelRequestRateLimitDuration(durationValue, durationExists, legacyMinutes, activated, activationAt)
	if err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	if !durationExists {
		common.OptionMap[setting.ModelRequestRateLimitDurationOption] = resolved
	}
	common.OptionMap[setting.ModelRequestRateLimitDurationActivatedOption] = strconv.FormatBool(activated)
	common.OptionMap[setting.ModelRequestRateLimitDurationActivationAtOption] = strconv.FormatInt(activationAt, 10)
	common.OptionMap[setting.ModelRequestRateLimitDurationActiveOption] = strconv.FormatBool(setting.ModelRequestRateLimitDurationConfig().Canonical)
	common.OptionMap[setting.ModelRequestRateLimitDurationStagedOption] = strconv.FormatBool(durationExists)
	common.OptionMapRWMutex.Unlock()
	return nil
}

// RefreshModelRequestRateLimitDurationMetadata keeps the API-visible active
// state aligned with the shared cutover timestamp without querying the DB.
func RefreshModelRequestRateLimitDurationMetadata() {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap[setting.ModelRequestRateLimitDurationActiveOption] = strconv.FormatBool(setting.ModelRequestRateLimitDurationConfig().Canonical)
}

func validateModelRequestRateLimitDurationActivation(values map[string]string) error {
	if values[setting.ModelRequestRateLimitDurationActivatedOption] != "true" {
		return nil
	}
	durationValue := values[setting.ModelRequestRateLimitDurationOption]
	if durationValue == "" {
		var option Option
		if err := DB.First(&option, "key = ?", setting.ModelRequestRateLimitDurationOption).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("model request rate limit duration must be staged before activation")
			}
			return err
		}
		durationValue = option.Value
	}
	if _, err := setting.ParseModelRequestRateLimitDuration(durationValue); err != nil {
		return errors.New("model request rate limit duration must be staged before activation")
	}
	return nil
}

func migratePricingGroupsFromLegacy(legacyGroupRatio string, legacyUsableGroups map[string]string) (bool, error) {
	value, err := ratio_setting.PricingGroupsFromLegacy(legacyGroupRatio, legacyUsableGroups)
	if err != nil {
		return false, err
	}
	option := Option{Key: "PricingGroups", Value: value}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return false, err
	}
	if err := DB.First(&option, "key = ?", "PricingGroups").Error; err != nil {
		return false, err
	}
	if err := updateOptionMapFromDatabase(option.Key, option.Value); err != nil {
		return false, err
	}
	return true, nil
}

func normalizePricingGroupOptionMaps() {
	previousAutoGroups := setting.AutoGroups2JsonString()
	values := map[string]string{
		"PricingGroups":   ratio_setting.PricingGroups2JSONString(),
		"GroupRatio":      ratio_setting.GroupRatio2JSONString(),
		"GroupGroupRatio": ratio_setting.GroupGroupRatio2JSONString(),
	}

	if normalized, err := ratio_setting.NormalizeGroupGroupRatio(); err == nil {
		values["GroupGroupRatio"] = normalized
	} else {
		common.SysLog("failed to normalize group-group ratios: " + err.Error())
	}
	if normalized, err := ratio_setting.NormalizeGroupSpecialUsableGroup(); err == nil {
		values["group_ratio_setting.group_special_usable_group"] = normalized
	} else {
		common.SysLog("failed to normalize special usable groups: " + err.Error())
	}
	if normalized, err := ratio_setting.NormalizeAutoGroups(); err == nil {
		values["AutoGroups"] = normalized
		if normalized != previousAutoGroups {
			if err := DB.Model(&Option{}).Where("key = ?", "AutoGroups").Update("value", normalized).Error; err != nil {
				common.SysLog("failed to persist recovered auto groups: " + err.Error())
			}
		}
	} else {
		common.SysLog("failed to normalize auto groups: " + err.Error())
	}

	common.OptionMapRWMutex.Lock()
	for key, value := range values {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
}

func optionLoadPriority(key string) int {
	switch key {
	case "AutoGroups", "GroupGroupRatio", "group_ratio_setting.group_special_usable_group":
		return 0
	case "GroupRatio":
		return 1
	case "PricingGroups":
		return 2
	default:
		return 1
	}
}

func ensureYooKassaPayMethodPersisted() error {
	values := make(map[string]string)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return ensurePaymentMethodsPersistedTx(tx, values)
	}); err != nil {
		return err
	}
	if value, ok := values["PayMethods"]; ok {
		return updateOptionMapFromDatabase("PayMethods", value)
	}
	return nil
}

func ensureNOWPaymentsPayMethodPersisted() error {
	values := make(map[string]string)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return ensurePaymentMethodsPersistedTx(tx, values)
	}); err != nil {
		return err
	}
	if value, ok := values["PayMethods"]; ok {
		return updateOptionMapFromDatabase("PayMethods", value)
	}
	return nil
}

func SyncOptions(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing options from database")
		loadOptionsFromDatabase()
	}
}

func UpdateOption(key string, value string) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionLocked(key, value, nil)
}

// UpdateOptionWithPaymentCurrencyGuard applies an option update while holding
// the shared platform-currency guard. Controllers use the callback to
// re-check provider readiness after acquiring the lock, closing the race with
// an administrator disabling or deleting that currency.
func UpdateOptionWithPaymentCurrencyGuard(key, value string, guard func() error) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionLocked(key, value, guard)
}

func UpdateOptionWithPaymentCurrencyTxGuard(key, value string, guard func(*gorm.DB) error) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionLockedWithTxGuard(key, value, guard)
}

func updateOptionLocked(key string, value string, paymentCurrencyGuard func() error) error {
	if paymentCurrencyGuard == nil {
		return updateOptionLockedWithTxGuard(key, value, nil)
	}
	return updateOptionLockedWithTxGuard(key, value, func(*gorm.DB) error { return paymentCurrencyGuard() })
}

func updateOptionLockedWithTxGuard(key string, value string, paymentCurrencyGuard func(*gorm.DB) error) error {

	if key == removedChatsOptionKey {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, key)
		common.OptionMapRWMutex.Unlock()
		return nil
	}
	if key == "UserUsableGroups" {
		return nil
	}
	if isCreemConfigOption(key) {
		return errors.New("Creem configuration must be updated atomically via UpdateOptionsBulk")
	}
	if err := validateOptionValue(key, value); err != nil {
		return err
	}
	if strings.HasPrefix(key, "USDTTRC20") {
		if err := validateDirectUSDTOptionValues(map[string]string{key: value}); err != nil {
			return err
		}
	}
	normalizedValue, err := normalizeOptionValueForSave(key, value)
	if err != nil {
		return err
	}
	value = normalizedValue
	if key == "GroupRatio" {
		key = "PricingGroups"
	}
	modelRequestRateLimitDurationChanged := key == setting.ModelRequestRateLimitDurationOption ||
		key == setting.ModelRequestRateLimitDurationLegacyOption ||
		key == setting.ModelRequestRateLimitDurationActivatedOption ||
		key == setting.ModelRequestRateLimitDurationActivationAtOption
	activationWasEnabled := false
	if key == setting.ModelRequestRateLimitDurationActivatedOption {
		var option Option
		err := DB.Select("value").First(&option, "key = ?", setting.ModelRequestRateLimitDurationActivatedOption).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		activationWasEnabled = err == nil && option.Value == "true"
		if value == "false" && activationWasEnabled {
			return errors.New("model request rate limit duration activation cannot be disabled")
		}
		if value == "true" && activationWasEnabled {
			return nil
		}
	}
	if err := validateModelRequestRateLimitDurationActivation(map[string]string{key: value}); err != nil {
		return err
	}
	if err := normalizePricingGroupReferencesBeforeOptionUpdate(key); err != nil {
		return err
	}
	values := map[string]string{key: value}
	if key == setting.ModelRequestRateLimitDurationActivatedOption && value == "true" && !activationWasEnabled {
		values[setting.ModelRequestRateLimitDurationActivationAtOption] = strconv.FormatInt(modelRequestRateLimitActivationNow()+modelRequestRateLimitActivationDelay(), 10)
	}
	if err := persistOptionsAndRuntimeWithTxGuard(values, paymentCurrencyGuard, nil); err != nil {
		return err
	}
	if modelRequestRateLimitDurationChanged {
		return refreshModelRequestRateLimitDuration()
	}
	return nil
}

func normalizePricingGroupReferencesBeforeOptionUpdate(key string) error {
	switch key {
	case "GroupRatio", "PricingGroups":
		if err := normalizePricingGroupOptionReferencesBeforeRename(); err != nil {
			return err
		}
		return normalizePricingGroupReferencesBeforeUpdate()
	default:
		return nil
	}
}

func normalizePricingGroupOptionReferencesBeforeRename() error {
	normalizers := map[string]func(string) (string, error){
		"AutoGroups":      ratio_setting.NormalizeAutoGroupsJSONString,
		"GroupGroupRatio": ratio_setting.NormalizeGroupGroupRatioJSONString,
		"group_ratio_setting.group_special_usable_group": ratio_setting.NormalizeGroupSpecialUsableGroupJSONString,
	}
	for key, normalize := range normalizers {
		var option Option
		err := DB.First(&option, "key = ?", key).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		normalized, err := normalize(option.Value)
		if err != nil {
			return err
		}
		if normalized == option.Value {
			continue
		}
		if err := DB.Model(&Option{}).Where("key = ?", key).Update("value", normalized).Error; err != nil {
			return err
		}
		if err := updateOptionMap(key, normalized); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionValue(key string, value string) error {
	switch key {
	case "PayMethods":
		methods, err := operation_setting.ParsePayMethodsJSON(value)
		if err != nil {
			return errors.New("PayMethods must be valid JSON")
		}
		return operation_setting.ValidatePayMethods(methods)
	case "payment_setting.amount_cashback":
		var cashbacks operation_setting.AmountCashbackConfig
		if err := common.Unmarshal([]byte(value), &cashbacks); err != nil {
			return err
		}
		return operation_setting.ValidateAmountCashback(cashbacks)
	case "ReferralDepositPercent":
		percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
			return errors.New("referral deposit percent must be between 0 and 100")
		}
		return nil
	case deprecatedReferralCashbackOption:
		return errors.New("ReferralCashbackPercent is no longer supported; configure referral cashback in payment_setting.amount_cashback")
	case "ReferralRequiredTopUpUSD":
		amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			return errors.New("referral required top-up USD must be greater than zero")
		}
		return nil
	case "QuotaPerUnit":
		quotaPerUnit, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) || quotaPerUnit <= 0 {
			return errors.New("quota per unit must be greater than zero")
		}
		return nil
	case operation_setting.PaymentPendingTTLMinutes:
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || valueInt <= 0 || valueInt > operation_setting.MaxPaymentPendingTTLMinutes {
			return fmt.Errorf("payment pending TTL must be between 1 and %d minutes", operation_setting.MaxPaymentPendingTTLMinutes)
		}
		return nil
	case operation_setting.PaymentCreationRateLimit:
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || valueInt <= 0 || valueInt > operation_setting.MaxPaymentCreationRateLimit {
			return fmt.Errorf("payment creation rate limit must be between 1 and %d", operation_setting.MaxPaymentCreationRateLimit)
		}
		return nil
	case operation_setting.PaymentCreationRateLimitDurationMinutes:
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || valueInt <= 0 || valueInt > operation_setting.MaxPaymentCreationRateLimitWindowMinutes {
			return fmt.Errorf("payment creation rate limit window must be between 1 and %d minutes", operation_setting.MaxPaymentCreationRateLimitWindowMinutes)
		}
		return nil
	case "USDTTRC20Enabled":
		if value != "true" && value != "false" {
			return errors.New("USDT TRC20 enabled must be true or false")
		}
		return nil
	case "USDTTRC20ReceivingAddress":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return setting.ValidateTRONAddress(value)
	case "USDTTONReceivingAddress":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return setting.ValidateTONAddress(value)
	case "USDTSolanaReceivingAddress":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return setting.ValidateSolanaAddress(value)
	case "USDTTONAPIKey", "USDTSolanaAPIKey":
		return nil
	case "USDTSolanaReceivingTokenAccount":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return setting.ValidateSolanaAddress(value)
	case "USDTTONAPIBaseURL", "USDTSolanaRPCURL":
		if strings.TrimSpace(value) == "" {
			return errors.New("RPC/API endpoint is required")
		}
		network := "TON"
		if key == "USDTSolanaRPCURL" {
			network = "SOLANA"
		}
		return setting.ValidateUSDTProviderEndpoint(network, value)
	case "USDTTRC20MaxCreationsPerHour":
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || valueInt < 0 || valueInt > setting.USDTTRC20MaxCreations {
			return fmt.Errorf("USDT TRC20 hourly creation limit must be between 0 and %d", setting.USDTTRC20MaxCreations)
		}
		return nil
	case "USDTTRC20AmountPrecision":
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return errors.New("USDT TRC20 amount precision must be an integer")
		}
		return setting.ValidateUSDTTRC20AmountPrecision(valueInt)
	case "USDTTRC20AmountTailLimitUnits":
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return errors.New("USDT TRC20 amount tail limit must be an integer")
		}
		return setting.ValidateUSDTTRC20AmountTailLimit(valueInt)
	case "USDTTRC20AmountSuffixMinUnits", "USDTTRC20AmountSuffixMaxUnits":
		// Deprecated compatibility options. Do not validate their historical
		// values: they are intentionally ignored by the runtime.
		return nil
	case setting.ModelRequestRateLimitDurationStagedOption,
		setting.ModelRequestRateLimitDurationActiveOption,
		setting.ModelRequestRateLimitDurationActivationAtOption:
		return errors.New("model request rate limit duration metadata is read-only")
	case setting.ModelRequestRateLimitDurationOption:
		_, err := setting.ParseModelRequestRateLimitDuration(value)
		return err
	case setting.ModelRequestRateLimitDurationActivatedOption:
		if value != "true" && value != "false" {
			return errors.New("model request rate limit duration activation must be true or false")
		}
		return nil
	case "ModelRequestRateLimitCount":
		return setting.ValidateModelRequestRateLimitCount(value)
	case "GroupRatio", "PricingGroups":
		if err := ratio_setting.ValidatePricingGroupsJSONString(value); err != nil {
			return err
		}
		return ratio_setting.ValidatePricingGroupIDStabilityJSONString(value)
	case "AutoGroups":
		return ratio_setting.ValidateAutoGroupsJSONString(value)
	case "billing_setting.task_price_unit":
		values := make(map[string]string)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return err
		}
		for _, unit := range values {
			if !billing_setting.IsTaskPriceUnit(unit) {
				return errors.New("unsupported task price unit")
			}
		}
		return nil
	default:
		return nil
	}
}

func isReferralCashbackOption(key string) bool {
	return key == "payment_setting.amount_cashback"
}

func validateReferralCashbackOptionValue(key, value string) error {
	switch key {
	case "payment_setting.amount_cashback":
		var cashbacks operation_setting.AmountCashbackConfig
		if err := common.Unmarshal([]byte(value), &cashbacks); err != nil {
			return err
		}
		return operation_setting.ValidateAmountCashback(cashbacks)
	default:
		return nil
	}
}

func validateDirectUSDTOptionValues(values map[string]string) error {
	enabled := setting.USDTTRC20Enabled
	address := setting.USDTTRC20ReceivingAddress
	apiKey := setting.USDTTRC20APIKey
	if value, ok := values["USDTTRC20Enabled"]; ok {
		enabled = value == "true"
	}
	if value, ok := values["USDTTRC20ReceivingAddress"]; ok {
		address = value
	}
	if value, ok := values["USDTTRC20APIKey"]; ok {
		apiKey = value
	}
	return setting.ValidateDirectUSDTConfigValues(enabled, address, apiKey)
}

func ensureDirectUSDTAmountTailLimitPersisted(limit int) (int, error) {
	if err := setting.ValidateUSDTTRC20AmountTailLimit(limit); err != nil {
		return 0, err
	}
	option := Option{Key: "USDTTRC20AmountTailLimitUnits", Value: strconv.Itoa(limit)}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return 0, err
	}
	if err := DB.First(&option, "key = ?", option.Key).Error; err != nil {
		return 0, err
	}
	persistedLimit, err := strconv.Atoi(strings.TrimSpace(option.Value))
	if err != nil {
		return 0, err
	}
	if err := setting.ValidateUSDTTRC20AmountTailLimit(persistedLimit); err != nil {
		return 0, err
	}
	return persistedLimit, nil
}

func ensureDirectUSDTAmountTailLimitOptionTx(tx *gorm.DB) error {
	option := Option{Key: "USDTTRC20AmountTailLimitUnits", Value: strconv.Itoa(setting.DefaultUSDTTRC20AmountTailLimitUnits)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return err
	}
	query := tx
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return query.First(&option, "key = ?", option.Key).Error
}

func validateDirectUSDTAmountTailLimitOptionValuesFromDB(tx *gorm.DB, values map[string]string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	limit := setting.DefaultUSDTTRC20AmountTailLimitUnits
	query := tx.Where("key = ?", "USDTTRC20AmountTailLimitUnits")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var option Option
	if err := query.First(&option).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if option.Key != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil {
			return fmt.Errorf("invalid persisted %s: %w", option.Key, err)
		}
		limit = parsed
	}
	if value, ok := values["USDTTRC20AmountTailLimitUnits"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		limit = parsed
	}
	return setting.ValidateUSDTTRC20AmountTailLimit(limit)
}

// ensureDirectUSDTAmountPrecisionOptionTx materializes the legacy precision
// row for old admin clients. New runtime configuration is tail-limit based.
func ensureDirectUSDTAmountPrecisionOptionTx(tx *gorm.DB) error {
	option := Option{Key: "USDTTRC20AmountPrecision", Value: strconv.Itoa(setting.DefaultUSDTTRC20AmountPrecision)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return err
	}
	query := tx
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return query.First(&option, "key = ?", option.Key).Error
}

// validateDirectUSDTAmountPrecisionOptionValuesFromDB validates a prospective
// precision update against the latest persisted value while holding a row lock
// on databases that support it.
func validateDirectUSDTAmountPrecisionOptionValuesFromDB(tx *gorm.DB, values map[string]string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	precision := setting.DefaultUSDTTRC20AmountPrecision
	query := tx.Where("key = ?", "USDTTRC20AmountPrecision")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var option Option
	if err := query.First(&option).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if option.Key != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil {
			return fmt.Errorf("invalid persisted %s: %w", option.Key, err)
		}
		precision = parsed
	}
	if value, ok := values["USDTTRC20AmountPrecision"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		precision = parsed
	}
	return setting.ValidateUSDTTRC20AmountPrecision(precision)
}

// Deprecated suffix-bound helpers are retained solely for old admin clients
// that still submit these keys. They never participate in payment runtime
// selection; the precision option above is authoritative.
func ensureDirectUSDTAmountSuffixOptionsTx(tx *gorm.DB) error {
	defaults := []struct{ key, value string }{
		{"USDTTRC20AmountSuffixMinUnits", strconv.Itoa(setting.DefaultUSDTTRC20AmountSuffixMinUnits)},
		{"USDTTRC20AmountSuffixMaxUnits", strconv.Itoa(setting.DefaultUSDTTRC20AmountSuffixMaxUnits)},
	}
	for _, item := range defaults {
		option := Option{Key: item.key, Value: item.value}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
			return err
		}
		query := tx
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&option, "key = ?", item.key).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateDirectUSDTAmountSuffixOptionValuesFromDB(tx *gorm.DB, values map[string]string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	minUnits := setting.DefaultUSDTTRC20AmountSuffixMinUnits
	maxUnits := setting.DefaultUSDTTRC20AmountSuffixMaxUnits
	query := tx.Where("key IN ?", []string{"USDTTRC20AmountSuffixMinUnits", "USDTTRC20AmountSuffixMaxUnits"})
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var options []Option
	if err := query.Find(&options).Error; err != nil {
		return err
	}
	for _, option := range options {
		parsed, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil {
			return fmt.Errorf("invalid persisted %s: %w", option.Key, err)
		}
		switch option.Key {
		case "USDTTRC20AmountSuffixMinUnits":
			minUnits = parsed
		case "USDTTRC20AmountSuffixMaxUnits":
			maxUnits = parsed
		}
	}
	if value, ok := values["USDTTRC20AmountSuffixMinUnits"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		minUnits = parsed
	}
	if value, ok := values["USDTTRC20AmountSuffixMaxUnits"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		maxUnits = parsed
	}
	return setting.ValidateUSDTTRC20AmountSuffixRange(minUnits, maxUnits)
}

func normalizeOptionValueForSave(key string, value string) (string, error) {
	var (
		normalized string
		ok         bool
		err        error
	)
	switch key {
	case "USDTTRC20AmountPrecision", "USDTTRC20AmountTailLimitUnits":
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			return value, parseErr
		}
		return strconv.Itoa(parsed), nil
	case "USDTTRC20ReceivingAddress", "USDTTONReceivingAddress", "USDTSolanaReceivingAddress":
		return strings.TrimSpace(value), nil
	case "payment_setting.amount_cashback":
		var cashbacks operation_setting.AmountCashbackConfig
		if err := common.Unmarshal([]byte(value), &cashbacks); err != nil {
			return value, err
		}
		encoded, err := common.Marshal(cashbacks)
		if err != nil {
			return value, err
		}
		return string(encoded), nil
	case setting.ModelRequestRateLimitDurationLegacyOption:
		return strings.TrimSuffix(setting.ResolveModelRequestRateLimitDuration("", false, value), "m"), nil
	case "GroupRatio":
		normalized, ok, err = ratio_setting.NormalizeGroupRatioJSONStringForSaveIfInitialized(value)
	case "PricingGroups":
		normalized, ok, err = ratio_setting.NormalizePricingGroupsJSONStringIfInitialized(value)
	case "AutoGroups":
		normalized, ok, err = ratio_setting.NormalizeAutoGroupsJSONStringIfInitialized(value)
	case "GroupGroupRatio":
		normalized, ok, err = ratio_setting.NormalizeGroupGroupRatioJSONStringIfInitialized(value)
	case "group_ratio_setting.group_special_usable_group":
		normalized, ok, err = ratio_setting.NormalizeGroupSpecialUsableGroupJSONStringIfInitialized(value)
	default:
		return value, nil
	}
	if err != nil || !ok {
		return value, err
	}
	return normalized, nil
}

// UpdateOptionsBulk persists related options in one transaction and restores
// the previous runtime values if applying or committing the update fails.
func UpdateOptionsBulk(values map[string]string) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionsBulkLocked(values, nil)
}

func UpdateOptionsBulkWithPaymentCurrencyGuard(values map[string]string, guard func() error) error {
	if guard == nil {
		return UpdateOptionsBulk(values)
	}
	return UpdateOptionsBulkWithPaymentCurrencyTxGuard(values, func(*gorm.DB) error { return guard() })
}

// UpdateOptionsBulkWithPaymentCurrencyTxGuard runs the readiness callback on
// the same transaction that owns the USD currency mutex.
func UpdateOptionsBulkWithPaymentCurrencyTxGuard(values map[string]string, guard func(*gorm.DB) error) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionsBulkLockedWithPrepareTx(values, nil, guard)
}

// UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepare applies a related option
// update after prepare has merged its values under the database transaction.
// The callback is intended for cross-instance read/merge/write flows: it can
// lock the rows it reads and update values before the final readiness guard
// and option writes run.
func UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepare(values map[string]string, prepare func(*gorm.DB, map[string]string) error, guard func() error) error {
	if guard == nil {
		return UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepareTx(values, prepare, nil)
	}
	return UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepareTx(values, prepare, func(*gorm.DB) error { return guard() })
}

// UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepareTx is the transaction-
// aware variant used by cross-replica payment readiness checks.
func UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepareTx(values map[string]string, prepare func(*gorm.DB, map[string]string) error, guard func(*gorm.DB) error) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	return updateOptionsBulkLockedWithPrepareTx(values, prepare, guard)
}

// ApplyJSONOptionPatches merges model-pricing JSON-object changes with the
// latest values stored in the database and persists all resulting options
// together. A missing option is treated as an empty JSON object.
func ApplyJSONOptionPatches(patches map[string]JSONObjectPatch) error {
	return ApplyJSONOptionPatchesWithTx(patches, nil)
}

// ApplyJSONOptionPatchesWithTx persists pricing patches and related metadata in
// one database transaction. The callback must only write database state; the
// in-memory pricing maps are refreshed after the transaction commits.
func ApplyJSONOptionPatchesWithTx(patches map[string]JSONObjectPatch, update func(*gorm.DB) error) error {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()

	if len(patches) == 0 && update == nil {
		return nil
	}
	var (
		values map[string]string
		keys   []string
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		values, keys, err = applyJSONOptionPatchesInTx(tx, patches, update)
		return err
	})
	if err != nil {
		return err
	}
	return refreshJSONOptionPatchRuntime(keys, values)
}

func applyJSONOptionPatchesInTx(tx *gorm.DB, patches map[string]JSONObjectPatch, update func(*gorm.DB) error) (map[string]string, []string, error) {
	values := make(map[string]string, len(patches))
	keys := make([]string, 0, len(patches))
	for key := range patches {
		if _, ok := jsonObjectPatchOptionKeys[key]; !ok {
			return nil, nil, errors.New("JSON option patch is only supported for model pricing options: " + key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	options := make(map[string]Option, len(keys))
	for _, optionKey := range keys {
		option := Option{Key: optionKey, Value: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", optionKey).Error; err != nil {
			return nil, nil, err
		}

		current := make(map[string]any)
		if err := common.UnmarshalJsonStr(option.Value, &current); err != nil {
			return nil, nil, err
		}
		if current == nil {
			return nil, nil, errors.New("option " + optionKey + " must contain a JSON object")
		}
		patch := patches[optionKey]
		for _, field := range patch.Delete {
			delete(current, field)
		}
		for field, value := range patch.Set {
			current[field] = value
		}
		encoded, err := common.Marshal(current)
		if err != nil {
			return nil, nil, err
		}
		value := string(encoded)
		if err := validateOptionValue(optionKey, value); err != nil {
			return nil, nil, err
		}
		value, err = normalizeOptionValueForSave(optionKey, value)
		if err != nil {
			return nil, nil, err
		}
		option.Value = value
		options[optionKey] = option
		values[optionKey] = value
	}
	for _, optionKey := range keys {
		option := options[optionKey]
		if err := tx.Save(&option).Error; err != nil {
			return nil, nil, err
		}
	}
	if update != nil {
		if err := update(tx); err != nil {
			return nil, nil, err
		}
	}
	return values, keys, nil
}

func refreshJSONOptionPatchRuntime(keys []string, values map[string]string) error {
	unlockPricing := ratio_setting.LockPricingConfigWrite()
	defer unlockPricing()
	for _, optionKey := range keys {
		if err := updateOptionMapFromDatabase(optionKey, values[optionKey]); err != nil {
			return err
		}
	}
	return nil
}

func UpdatePricingOptionManual(key, value string) error {
	if _, ok := jsonObjectPatchOptionKeys[key]; !ok {
		return errors.New("manual pricing update requires a model pricing option")
	}
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()

	if err := validateOptionValue(key, value); err != nil {
		return err
	}
	switch key {
	case "ModelRatio", "ModelPrice", "CompletionRatio", "CacheRatio", "CreateCacheRatio",
		"ImageRatio", "AudioRatio", "AudioCompletionRatio":
		values := make(map[string]float64)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return err
		}
	case "billing_setting.billing_mode":
		values := make(map[string]string)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return err
		}
		for _, mode := range values {
			if mode != billing_setting.BillingModeRatio && mode != billing_setting.BillingModeTieredExpr {
				return errors.New("unsupported billing mode")
			}
		}
	case "billing_setting.billing_expr":
		values := make(map[string]string)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return err
		}
		for _, expr := range values {
			if strings.TrimSpace(expr) == "" || billing_setting.SmokeTestExpr(expr) != nil {
				return errors.New("invalid billing expression")
			}
		}
	}
	normalized, err := normalizeOptionValueForSave(key, value)
	if err != nil {
		return err
	}
	value = normalized
	err = DB.Transaction(func(tx *gorm.DB) error {
		option := Option{Key: key, Value: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", key).Error; err != nil {
			return err
		}
		before := make(map[string]any)
		after := make(map[string]any)
		if err := common.UnmarshalJsonStr(option.Value, &before); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(value, &after); err != nil {
			return err
		}
		changed := make(map[string]struct{})
		for name, previous := range before {
			if next, ok := after[name]; !ok || common.Interface2String(next) != common.Interface2String(previous) {
				changed[name] = struct{}{}
			}
		}
		for name, next := range after {
			if previous, ok := before[name]; !ok || common.Interface2String(next) != common.Interface2String(previous) {
				changed[name] = struct{}{}
			}
		}
		option.Value = value
		if err := tx.Save(&option).Error; err != nil {
			return err
		}
		if len(changed) > 0 {
			if err := bumpPricingSyncConfigVersionTx(tx); err != nil {
				return err
			}
		}
		for modelName := range changed {
			state := PricingSyncModelState{ModelName: modelName, Mode: PricingSyncModelModeManual, Status: PricingSyncModelStatusReady}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "model_name"}},
				DoUpdates: clause.Assignments(map[string]any{
					"mode": PricingSyncModelModeManual, "channel_id": 0,
					"provenance": "", "conflict_details": "", "status": PricingSyncModelStatusReady,
				}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	unlockPricing := ratio_setting.LockPricingConfigWrite()
	defer unlockPricing()
	return updateOptionMapFromDatabase(key, value)
}

func updateOptionsBulkLocked(values map[string]string, paymentCurrencyGuard func() error) error {
	if paymentCurrencyGuard == nil {
		return updateOptionsBulkLockedWithPrepareTx(values, nil, nil)
	}
	return updateOptionsBulkLockedWithPrepareTx(values, nil, func(*gorm.DB) error { return paymentCurrencyGuard() })
}

func updateOptionsBulkLockedWithPrepare(values map[string]string, paymentCurrencyGuard func() error, prepare func(*gorm.DB, map[string]string) error) error {
	if paymentCurrencyGuard == nil {
		return updateOptionsBulkLockedWithPrepareTx(values, prepare, nil)
	}
	return updateOptionsBulkLockedWithPrepareTx(values, prepare, func(*gorm.DB) error { return paymentCurrencyGuard() })
}

func updateOptionsBulkLockedWithPrepareTx(values map[string]string, prepare func(*gorm.DB, map[string]string) error, paymentCurrencyGuard func(*gorm.DB) error) error {
	if len(values) == 0 {
		return nil
	}
	_, removedChats := values[removedChatsOptionKey]
	if removedChats {
		filtered := make(map[string]string, len(values)-1)
		for key, value := range values {
			if key != removedChatsOptionKey {
				filtered[key] = value
			}
		}
		values = filtered
		if len(values) == 0 {
			common.OptionMapRWMutex.Lock()
			delete(common.OptionMap, removedChatsOptionKey)
			common.OptionMapRWMutex.Unlock()
			return nil
		}
	}
	activationWasEnabled := false
	if values[setting.ModelRequestRateLimitDurationActivatedOption] == "false" || values[setting.ModelRequestRateLimitDurationActivatedOption] == "true" {
		var option Option
		err := DB.Select("value").First(&option, "key = ?", setting.ModelRequestRateLimitDurationActivatedOption).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		activationWasEnabled = err == nil && option.Value == "true"
		if values[setting.ModelRequestRateLimitDurationActivatedOption] == "false" && activationWasEnabled {
			return errors.New("model request rate limit duration activation cannot be disabled")
		}
	}
	prospectivePricingGroups := values["PricingGroups"]
	if prospectivePricingGroups == "" {
		prospectivePricingGroups = values["GroupRatio"]
	}
	normalizedValues := make(map[string]string, len(values))
	for _, k := range sortedOptionKeys(values) {
		v := values[k]
		if k == "UserUsableGroups" {
			continue
		}
		if isReferralCashbackOption(k) {
			if err := validateReferralCashbackOptionValue(k, v); err != nil {
				return err
			}
			normalized, err := normalizeOptionValueForSave(k, v)
			if err != nil {
				return err
			}
			normalizedValues[k] = normalized
			continue
		}
		if k == "AutoGroups" && prospectivePricingGroups != "" {
			normalized, err := ratio_setting.NormalizeAutoGroupsJSONStringForPricingGroups(v, prospectivePricingGroups)
			if err != nil {
				return err
			}
			normalizedValues[k] = normalized
			continue
		}
		if err := validateOptionValue(k, v); err != nil {
			return err
		}
		normalized, err := normalizeOptionValueForSave(k, v)
		if err != nil {
			return err
		}
		if k == "GroupRatio" {
			k = "PricingGroups"
		}
		normalizedValues[k] = normalized
	}
	for k := range normalizedValues {
		if err := normalizePricingGroupReferencesBeforeOptionUpdate(k); err != nil {
			return err
		}
	}
	if err := validateDirectUSDTOptionValues(normalizedValues); err != nil {
		return err
	}
	if err := validateModelRequestRateLimitDurationActivation(normalizedValues); err != nil {
		return err
	}
	if normalizedValues[setting.ModelRequestRateLimitDurationActivatedOption] == "true" && !activationWasEnabled {
		normalizedValues[setting.ModelRequestRateLimitDurationActivationAtOption] = strconv.FormatInt(modelRequestRateLimitActivationNow()+modelRequestRateLimitActivationDelay(), 10)
	}
	if err := persistOptionsAndRuntimeWithTxGuard(normalizedValues, paymentCurrencyGuard, prepare); err != nil {
		return err
	}
	if removedChats {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, removedChatsOptionKey)
		common.OptionMapRWMutex.Unlock()
	}
	if _, changed := normalizedValues[setting.ModelRequestRateLimitDurationOption]; changed {
		return refreshModelRequestRateLimitDuration()
	}
	if _, changed := normalizedValues[setting.ModelRequestRateLimitDurationLegacyOption]; changed {
		return refreshModelRequestRateLimitDuration()
	}
	if _, changed := normalizedValues[setting.ModelRequestRateLimitDurationActivatedOption]; changed {
		return refreshModelRequestRateLimitDuration()
	}
	return nil
}

func isCreemConfigOption(key string) bool {
	switch key {
	case "CreemApiKey", "CreemProducts", "CreemTestMode", "CreemWebhookSecret":
		return true
	default:
		return false
	}
}

func publishCreemConfig() {
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey: setting.CreemApiKey, Products: setting.CreemProducts,
		TestMode: setting.CreemTestMode, WebhookSecret: setting.CreemWebhookSecret,
	})
}

func isYooKassaConfigOption(key string) bool {
	switch key {
	case "YooKassaEnabled", "YooKassaShopID", "YooKassaSecretKey", "YooKassaReturnURL", "YooKassaPaymentMethods":
		return true
	default:
		return false
	}
}

func publishYooKassaConfig() {
	setting.PublishYooKassaConfig(setting.YooKassaConfig{
		Enabled:        setting.YooKassaEnabled,
		ShopID:         setting.YooKassaShopID,
		SecretKey:      setting.YooKassaSecretKey,
		ReturnURL:      setting.YooKassaReturnURL,
		PaymentMethods: setting.YooKassaPaymentMethods,
	})
}

func persistOptionsAndRuntime(values map[string]string, paymentCurrencyGuard func() error, prepare func(*gorm.DB, map[string]string) error) error {
	if paymentCurrencyGuard == nil {
		return persistOptionsAndRuntimeWithTxGuard(values, nil, prepare)
	}
	return persistOptionsAndRuntimeWithTxGuard(values, func(*gorm.DB) error { return paymentCurrencyGuard() }, prepare)
}

func persistOptionsAndRuntimeWithTxGuard(values map[string]string, paymentCurrencyGuard func(*gorm.DB) error, prepare func(*gorm.DB, map[string]string) error) error {
	keys := sortedOptionKeys(values)
	pricingNormalizer, pricingGroupsChanged, err := pricingGroupNormalizerForOptions(keys, values)
	if err != nil {
		return err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if payMethods, included := values["PayMethods"]; included {
			if err := validatePayMethodsForSaveTx(tx, payMethods); err != nil {
				return err
			}
		}
		if prepare != nil {
			if err := prepare(tx, values); err != nil {
				return err
			}
		}
		_, precisionUpdate := values["USDTTRC20AmountPrecision"]
		if precisionUpdate {
			if err := ensureDirectUSDTAmountPrecisionOptionTx(tx); err != nil {
				return err
			}
			if err := validateDirectUSDTAmountPrecisionOptionValuesFromDB(tx, values); err != nil {
				return err
			}
		}
		_, tailLimitUpdate := values["USDTTRC20AmountTailLimitUnits"]
		if tailLimitUpdate {
			if err := ensureDirectUSDTAmountTailLimitOptionTx(tx); err != nil {
				return err
			}
			if err := validateDirectUSDTAmountTailLimitOptionValuesFromDB(tx, values); err != nil {
				return err
			}
		}
		_, legacyMinUpdate := values["USDTTRC20AmountSuffixMinUnits"]
		_, legacyMaxUpdate := values["USDTTRC20AmountSuffixMaxUnits"]
		if legacyMinUpdate || legacyMaxUpdate {
			if err := ensureDirectUSDTAmountSuffixOptionsTx(tx); err != nil {
				return err
			}
			if err := validateDirectUSDTAmountSuffixOptionValuesFromDB(tx, values); err != nil {
				return err
			}
		}
		if paymentCurrencyGuard != nil {
			if err := lockPlatformCurrencyGuard(tx); err != nil {
				return err
			}
			if err := paymentCurrencyGuard(tx); err != nil {
				return err
			}
		}
		for _, key := range keys {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = values[key]
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		if err := ensurePaymentMethodsPersistedTx(tx, values); err != nil {
			return err
		}
		if !pricingGroupsChanged {
			return nil
		}
		if err := normalizeChannelPricingGroupsTxWith(tx, pricingNormalizer.normalizeCSV, pricingNormalizer.normalizeKey); err != nil {
			return err
		}
		if err := normalizeTokenPricingGroupsTxWith(tx, pricingNormalizer.normalizeKey, pricingNormalizer.normalizeCSV); err != nil {
			return err
		}
		return normalizeTaskPricingGroupsTxWith(tx, pricingNormalizer.normalizeKeyOrDefault)
	})
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := updateOptionMapFromDatabase(key, values[key]); err != nil {
			return err
		}
	}
	if _, included := values["PayMethods"]; included {
		seen := false
		for _, key := range keys {
			if key == "PayMethods" {
				seen = true
				break
			}
		}
		if !seen {
			if err := updateOptionMapFromDatabase("PayMethods", values["PayMethods"]); err != nil {
				return err
			}
		}
	}
	for _, key := range keys {
		if isCreemConfigOption(key) {
			publishCreemConfig()
			break
		}
	}
	for _, key := range keys {
		if isYooKassaConfigOption(key) {
			publishYooKassaConfig()
			break
		}
	}
	if pricingGroupsChanged {
		InvalidatePricingCache()
	}
	return nil
}

// validatePayMethodsForSaveTx compares a candidate catalog with the last
// committed row while holding its transaction lock. This keeps the legacy
// topup_group exception scoped to methods that were actually persisted before
// the field existed; a controller-only check could race or be bypassed.
func validatePayMethodsForSaveTx(tx *gorm.DB, candidateJSON string) error {
	candidate, err := operation_setting.ParsePayMethodsJSON(candidateJSON)
	if err != nil {
		return errors.New("PayMethods must be valid JSON")
	}
	query := tx.Where("key = ?", "PayMethods")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var previous Option
	err = query.First(&previous).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return operation_setting.ValidatePayMethodsForSave(candidate, nil)
	}
	if err != nil {
		return err
	}
	persisted, err := operation_setting.ParsePayMethodsJSON(previous.Value)
	if err != nil {
		return fmt.Errorf("invalid persisted PayMethods: %w", err)
	}
	return operation_setting.ValidatePayMethodsForSave(candidate, persisted)
}

// ensurePaymentMethodsPersistedTx performs provider migrations while the
// option update transaction is still open. It always starts from the latest
// PayMethods row (locked for update), so a stale process snapshot cannot
// overwrite edits made by another instance.
func ensurePaymentMethodsPersistedTx(tx *gorm.DB, values map[string]string) error {
	_, explicit := values["PayMethods"]
	directLegacyReady := directUSDTLegacyReadyForMigration(values)
	yooKassaConfig := setting.GetYooKassaConfig()
	yooEnabled := optionBoolValue(values, "YooKassaEnabled", yooKassaConfig.Enabled)
	yooShopID := optionStringValue(values, "YooKassaShopID", yooKassaConfig.ShopID)
	yooSecret := optionStringValue(values, "YooKassaSecretKey", yooKassaConfig.SecretKey)
	yooMethods := optionStringValue(values, "YooKassaPaymentMethods", yooKassaConfig.PaymentMethods)
	sbpEnabled := false
	for _, configured := range strings.Split(yooMethods, ",") {
		if strings.EqualFold(strings.TrimSpace(configured), "sbp") {
			sbpEnabled = true
			break
		}
	}
	yooReady := yooEnabled && strings.TrimSpace(yooShopID) != "" && strings.TrimSpace(yooSecret) != "" && operation_setting.IsPaymentComplianceConfirmed() && sbpEnabled
	localHasNOW := false
	for _, method := range operation_setting.PayMethodsSnapshot() {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), PaymentMethodNOWPayments) {
			localHasNOW = true
			break
		}
	}
	if !explicit && !yooReady && !localHasNOW && !directLegacyReady {
		return nil
	}
	option := Option{Key: "PayMethods", Value: operation_setting.PayMethods2JsonString()}
	dirty := false
	if !explicit {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", option.Key).Error; err != nil {
			return err
		}
	} else {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", option.Key).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			option.Value = values["PayMethods"]
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", option.Key).Error; err != nil {
				return err
			}
		}
	}
	methods, err := operation_setting.ParsePayMethodsJSON(option.Value)
	if err != nil {
		return err
	}
	canonicalMethods := operation_setting.CanonicalizePayMethods(methods)
	if encodedBefore, err := common.Marshal(methods); err != nil {
		return err
	} else if encodedAfter, err := common.Marshal(canonicalMethods); err != nil {
		return err
	} else if string(encodedBefore) != string(encodedAfter) {
		methods = canonicalMethods
		encoded, err := common.Marshal(methods)
		if err != nil {
			return err
		}
		option.Value = string(encoded)
		dirty = true
	} else {
		methods = canonicalMethods
	}

	if yooReady && !explicit {
		var changed bool
		methods, changed = operation_setting.EnsureYooKassaPayMethod(methods, true)
		if changed {
			encoded, err := common.Marshal(methods)
			if err != nil {
				return err
			}
			option.Value = string(encoded)
			dirty = true
		}
	}

	var directMigrationMarker Option
	markerErr := tx.Select("key").First(&directMigrationMarker, "key = ?", operation_setting.DirectUSDTTRC20PayMethodsMigratedOption).Error
	directMigrationDone := markerErr == nil
	if markerErr != nil && !errors.Is(markerErr, gorm.ErrRecordNotFound) {
		return markerErr
	}
	if directLegacyReady && !explicit && !directMigrationDone {
		if !HasDirectUSDTMethod(methods) {
			methods = append(methods, map[string]string{
				"name": "Crypto", "type": DirectUSDTTRC20Provider,
			})
			encoded, err := common.Marshal(methods)
			if err != nil {
				return err
			}
			option.Value = string(encoded)
			dirty = true
		}
		marker := Option{Key: operation_setting.DirectUSDTTRC20PayMethodsMigratedOption, Value: "true"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error; err != nil {
			return err
		}
	}

	// NOWPayments used to be persisted from the process-local list. If the
	// durable row already exists, retain it; only initialize a missing row from
	// the local snapshot.
	if strings.TrimSpace(option.Value) == "" {
		encoded, err := common.Marshal(methods)
		if err != nil {
			return err
		}
		option.Value = string(encoded)
	}
	if dirty {
		if err := tx.Save(&option).Error; err != nil {
			return err
		}
	}
	values["PayMethods"] = option.Value
	return nil
}

func optionStringValue(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func optionBoolValue(values map[string]string, key string, fallback bool) bool {
	value := optionStringValue(values, key, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func directUSDTLegacyReadyForMigration(values map[string]string) bool {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return false
	}
	enabled := optionBoolValue(values, "USDTTRC20Enabled", setting.USDTTRC20Enabled)
	address := optionStringValue(values, "USDTTRC20ReceivingAddress", setting.USDTTRC20ReceivingAddress)
	apiKey := optionStringValue(values, "USDTTRC20APIKey", setting.USDTTRC20APIKey)
	return setting.ValidateDirectUSDTConfigValues(enabled, address, apiKey) == nil && enabled
}

type pricingGroupNormalizer struct {
	normalizeKey          func(string) string
	normalizeKeyOrDefault func(string) string
	normalizeCSV          func(string) string
}

func pricingGroupNormalizerForOptions(keys []string, values map[string]string) (pricingGroupNormalizer, bool, error) {
	value := ""
	for _, key := range keys {
		if key == "GroupRatio" || key == "PricingGroups" {
			value = values[key]
		}
	}
	if value == "" {
		return pricingGroupNormalizer{}, false, nil
	}
	normalized, err := ratio_setting.NormalizePricingGroupsJSONString(value)
	if err != nil {
		return pricingGroupNormalizer{}, false, err
	}
	var groups []ratio_setting.PricingGroup
	if err := common.Unmarshal([]byte(normalized), &groups); err != nil {
		return pricingGroupNormalizer{}, false, err
	}
	keysByReference := make(map[string]string, len(groups)*2)
	defaultKey := "1"
	for _, group := range groups {
		id := strconv.Itoa(group.Id)
		keysByReference[group.Name] = id
		keysByReference[id] = id
		if group.Name == "default" {
			defaultKey = id
		}
	}
	normalizeKey := func(value string) string {
		value = strings.TrimSpace(value)
		if normalized, ok := keysByReference[value]; ok {
			return normalized
		}
		return value
	}
	normalizeKeyOrDefault := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "auto" {
			return value
		}
		if normalized, ok := keysByReference[value]; ok {
			return normalized
		}
		return defaultKey
	}
	normalizeCSV := func(value string) string {
		parts := strings.Split(strings.Trim(value, ","), ",")
		normalized := make([]string, 0, len(parts))
		seen := make(map[string]struct{}, len(parts))
		for _, part := range parts {
			key := normalizeKey(part)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			normalized = append(normalized, key)
		}
		return strings.Join(normalized, ",")
	}
	return pricingGroupNormalizer{
		normalizeKey:          normalizeKey,
		normalizeKeyOrDefault: normalizeKeyOrDefault,
		normalizeCSV:          normalizeCSV,
	}, true, nil
}

func sortedOptionKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		iPricing := keys[i] == "GroupRatio" || keys[i] == "PricingGroups"
		jPricing := keys[j] == "GroupRatio" || keys[j] == "PricingGroups"
		if iPricing != jPricing {
			return iPricing
		}
		return keys[i] < keys[j]
	})
	return keys
}

func updateOptionMapFromDatabase(key string, value string) error {
	if key == "QuotaPerUnit" {
		if err := validateOptionValue(key, value); err != nil {
			return err
		}
	}
	if key == "payment_setting.amount_cashback" {
		normalized, err := normalizeOptionValueForSave(key, value)
		if err != nil {
			return err
		}
		value = normalized
	}
	return updateOptionMapWithPricingReferenceNormalization(key, value, false)
}

func updateOptionMap(key string, value string) error {
	return updateOptionMapWithPricingReferenceNormalization(key, value, true)
}

func updateOptionMapWithPricingReferenceNormalization(key string, value string, normalizePricingReferences bool) (err error) {
	if key == removedChatsOptionKey {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, key)
		common.OptionMapRWMutex.Unlock()
		return nil
	}
	if key == "UserUsableGroups" {
		return nil
	}
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[key] = value

	// 检查是否是模型配置 - 使用更规范的方式处理
	if handleConfigUpdate(key, value) {
		return nil // 已由配置系统处理
	}

	// 处理传统配置项...
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		}
	}
	if strings.HasSuffix(key, "Enabled") || key == "DefaultCollapseSidebar" || key == "DefaultUseAutoGroup" || key == "SMTPForceAuthLogin" || key == "SMTPInsecureSkipVerify" {
		boolValue := value == "true"
		switch key {
		case "PasswordRegisterEnabled":
			common.PasswordRegisterEnabled = boolValue
		case "PasswordLoginEnabled":
			common.PasswordLoginEnabled = boolValue
		case "EmailVerificationEnabled":
			common.EmailVerificationEnabled = boolValue
		case "GitHubOAuthEnabled":
			common.GitHubOAuthEnabled = boolValue
		case "LinuxDOOAuthEnabled":
			common.LinuxDOOAuthEnabled = boolValue
		case "WeChatAuthEnabled":
			common.WeChatAuthEnabled = boolValue
		case "TelegramOAuthEnabled":
			common.TelegramOAuthEnabled = boolValue
		case "TurnstileCheckEnabled":
			common.TurnstileCheckEnabled = boolValue
		case "RegisterEnabled":
			common.RegisterEnabled = boolValue
		case "EmailDomainRestrictionEnabled":
			common.EmailDomainRestrictionEnabled = boolValue
		case "EmailAliasRestrictionEnabled":
			common.EmailAliasRestrictionEnabled = boolValue
		case "AutomaticDisableChannelEnabled":
			common.AutomaticDisableChannelEnabled = boolValue
		case "AutomaticEnableChannelEnabled":
			common.AutomaticEnableChannelEnabled = boolValue
		case "LogConsumeEnabled":
			common.LogConsumeEnabled = boolValue
		case "DisplayInCurrencyEnabled":
			// 兼容旧字段：同步到新配置 general_setting.quota_display_type（运行时生效）
			// true -> USD, false -> TOKENS
			newVal := "USD"
			if !boolValue {
				newVal = "TOKENS"
			}
			if cfg := config.GlobalConfig.Get("general_setting"); cfg != nil {
				_ = config.UpdateConfigFromMap(cfg, map[string]string{"quota_display_type": newVal})
			}
		case "DisplayTokenStatEnabled":
			common.DisplayTokenStatEnabled = boolValue
		case "DrawingEnabled":
			common.DrawingEnabled = boolValue
		case "TaskEnabled":
			common.TaskEnabled = boolValue
		case "DataExportEnabled":
			common.DataExportEnabled = boolValue
		case "DefaultCollapseSidebar":
			common.DefaultCollapseSidebar = boolValue
		case "MjNotifyEnabled":
			setting.MjNotifyEnabled = boolValue
		case "MjAccountFilterEnabled":
			setting.MjAccountFilterEnabled = boolValue
		case "MjModeClearEnabled":
			setting.MjModeClearEnabled = boolValue
		case "MjForwardUrlEnabled":
			setting.MjForwardUrlEnabled = boolValue
		case "MjActionCheckSuccessEnabled":
			setting.MjActionCheckSuccessEnabled = boolValue
		case "CheckSensitiveEnabled":
			setting.CheckSensitiveEnabled = boolValue
		case "DemoSiteEnabled":
			operation_setting.DemoSiteEnabled = boolValue
		case "SelfUseModeEnabled":
			operation_setting.SelfUseModeEnabled = boolValue
		case "CheckSensitiveOnPromptEnabled":
			setting.CheckSensitiveOnPromptEnabled = boolValue
		case "ModelRequestRateLimitEnabled":
			setting.ModelRequestRateLimitEnabled = boolValue
		case "StopOnSensitiveEnabled":
			setting.StopOnSensitiveEnabled = boolValue
		case "SMTPSSLEnabled":
			common.SMTPSSLEnabled = boolValue
		case "SMTPStartTLSEnabled":
			common.SMTPStartTLSEnabled = boolValue
		case "SMTPInsecureSkipVerify":
			common.SMTPInsecureSkipVerify = boolValue
		case "SMTPForceAuthLogin":
			common.SMTPForceAuthLogin = boolValue
		case "WorkerAllowHttpImageRequestEnabled":
			system_setting.WorkerAllowHttpImageRequestEnabled = boolValue
		case "DefaultUseAutoGroup":
			setting.DefaultUseAutoGroup = boolValue
		case "ExposeRatioEnabled":
			ratio_setting.SetExposeRatioEnabled(boolValue)
		case "USDTTRC20Enabled":
			setting.USDTTRC20Enabled = boolValue
		}
	}
	switch key {
	case "EmailDomainWhitelist":
		common.EmailDomainWhitelist = strings.Split(value, ",")
	case "SMTPServer":
		common.SMTPServer = value
	case "SMTPPort":
		intValue, _ := strconv.Atoi(value)
		common.SMTPPort = intValue
	case "SMTPAccount":
		common.SMTPAccount = value
	case "SMTPFrom":
		common.SMTPFrom = value
	case "SMTPToken":
		common.SMTPToken = value
	case "ServerAddress":
		system_setting.ServerAddress = value
	case "WorkerUrl":
		system_setting.WorkerUrl = value
	case "WorkerValidKey":
		system_setting.WorkerValidKey = value
	case "PayAddress":
		operation_setting.PayAddress = value
	case "AutoGroups":
		err = setting.UpdateAutoGroupsByJsonString(value)
		if err == nil {
			var normalized string
			var normalizedOK bool
			normalized, normalizedOK, err = ratio_setting.NormalizeAutoGroupsIfInitialized()
			if err == nil && normalizedOK {
				common.OptionMap[key] = normalized
			}
		}
	case "CustomCallbackAddress":
		operation_setting.CustomCallbackAddress = value
	case "EpayId":
		operation_setting.EpayId = value
	case "EpayKey":
		operation_setting.EpayKey = value
	case "Price":
		operation_setting.Price, _ = strconv.ParseFloat(value, 64)
	case "USDExchangeRate":
		operation_setting.USDExchangeRate, _ = strconv.ParseFloat(value, 64)
	case "MinTopUp":
		operation_setting.MinTopUp, _ = strconv.ParseFloat(value, 64)
	case "StripeApiSecret":
		setting.StripeApiSecret = value
	case "StripeWebhookSecret":
		setting.StripeWebhookSecret = value
	case "StripePriceId":
		setting.StripePriceId = value
	case "StripeUnitPrice":
		setting.StripeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "StripeMinTopUp":
		setting.StripeMinTopUp, _ = strconv.ParseFloat(value, 64)
	case "StripePromotionCodesEnabled":
		setting.StripePromotionCodesEnabled = value == "true"
	case "CreemApiKey":
		setting.CreemApiKey = value
	case "CreemProducts":
		setting.CreemProducts = value
	case "CreemTestMode":
		setting.CreemTestMode = value == "true"
	case "CreemWebhookSecret":
		setting.CreemWebhookSecret = value
	case "WaffoEnabled":
		setting.WaffoEnabled = value == "true"
	case "WaffoApiKey":
		setting.WaffoApiKey = value
	case "WaffoPrivateKey":
		setting.WaffoPrivateKey = value
	case "WaffoPublicCert":
		setting.WaffoPublicCert = value
	case "WaffoSandboxPublicCert":
		setting.WaffoSandboxPublicCert = value
	case "WaffoSandboxApiKey":
		setting.WaffoSandboxApiKey = value
	case "WaffoSandboxPrivateKey":
		setting.WaffoSandboxPrivateKey = value
	case "WaffoSandbox":
		setting.WaffoSandbox = value == "true"
	case "WaffoMerchantId":
		setting.WaffoMerchantId = value
	case "WaffoNotifyUrl":
		setting.WaffoNotifyUrl = value
	case "WaffoReturnUrl":
		setting.WaffoReturnUrl = value
	case "WaffoSubscriptionReturnUrl":
		setting.WaffoSubscriptionReturnUrl = value
	case "WaffoCurrency":
		setting.WaffoCurrency = value
	case "WaffoUnitPrice":
		setting.WaffoUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoMinTopUp":
		setting.WaffoMinTopUp, _ = strconv.ParseFloat(value, 64)
	case "WaffoPancakeMerchantID":
		setting.WaffoPancakeMerchantID = value
	case "WaffoPancakePrivateKey":
		setting.WaffoPancakePrivateKey = value
	case "WaffoPancakeReturnURL":
		setting.WaffoPancakeReturnURL = value
	case "WaffoPancakeStoreID":
		setting.WaffoPancakeStoreID = value
	case "WaffoPancakeProductID":
		setting.WaffoPancakeProductID = value
	case "WaffoPancakeUnitPrice":
		setting.WaffoPancakeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "WaffoPancakeMinTopUp":
		setting.WaffoPancakeMinTopUp, _ = strconv.ParseFloat(value, 64)
	case "YooKassaEnabled":
		setting.YooKassaEnabled = value == "true"
	case "YooKassaShopID":
		setting.YooKassaShopID = value
	case "YooKassaSecretKey":
		setting.YooKassaSecretKey = value
	case "YooKassaReturnURL":
		setting.YooKassaReturnURL = value
	case "YooKassaPaymentMethods":
		setting.YooKassaPaymentMethods = value
	case "NOWPaymentsEnabled":
		setting.NOWPaymentsEnabled = value == "true"
	case "NOWPaymentsAPIKey":
		setting.NOWPaymentsAPIKey = value
	case "NOWPaymentsIPNSecret":
		setting.NOWPaymentsIPNSecret = value
	case "NOWPaymentsPriceCurrency":
		setting.NOWPaymentsPriceCurrency = "usdt"
		common.OptionMap[key] = "usdt"
	case "NOWPaymentsPayCurrency":
		setting.NOWPaymentsPayCurrency = "usdt"
		common.OptionMap[key] = "usdt"
	case "NOWPaymentsIPNCallbackURL":
		setting.NOWPaymentsIPNCallbackURL = value
	case "USDTTRC20ReceivingAddress":
		setting.USDTTRC20ReceivingAddress = strings.TrimSpace(value)
	case "USDTTONReceivingAddress":
		setting.USDTTONReceivingAddress = strings.TrimSpace(value)
	case "USDTSolanaReceivingAddress":
		setting.USDTSolanaReceivingAddress = strings.TrimSpace(value)
	case "USDTSolanaReceivingTokenAccount":
		setting.USDTSolanaReceivingTokenAccount = strings.TrimSpace(value)
	case "USDTTRC20AmountPrecision":
		precision, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			return parseErr
		}
		if err = setting.ValidateUSDTTRC20AmountPrecision(precision); err != nil {
			return err
		}
		setting.USDTTRC20AmountPrecision = precision
	case "USDTTRC20AmountTailLimitUnits":
		limit, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil {
			return parseErr
		}
		if err = setting.ValidateUSDTTRC20AmountTailLimit(limit); err != nil {
			return err
		}
		setting.USDTTRC20AmountTailLimitUnits = limit
	case "USDTTRC20AmountSuffixMinUnits":
		// Deprecated compatibility option; ignored by the payment runtime.
	case "USDTTRC20AmountSuffixMaxUnits":
		// Deprecated compatibility option; ignored by the payment runtime.
	case "USDTTRC20APIKey":
		setting.USDTTRC20APIKey = strings.TrimSpace(value)
	case "USDTTONAPIKey":
		setting.USDTTONAPIKey = strings.TrimSpace(value)
	case "USDTTONAPIBaseURL":
		setting.USDTTONAPIBaseURL = strings.TrimSpace(value)
	case "USDTSolanaRPCURL":
		setting.USDTSolanaRPCURL = strings.TrimSpace(value)
	case "USDTSolanaAPIKey":
		setting.USDTSolanaAPIKey = strings.TrimSpace(value)
	case "USDTTRC20MaxCreationsPerHour":
		setting.USDTTRC20MaxCreationsPerHour, _ = strconv.Atoi(value)
	case "USDTTRC20PaymentURLBase":
		setting.USDTTRC20PaymentURLBase = strings.TrimRight(strings.TrimSpace(value), "/")
	case "TopupGroupRatio":
		err = common.UpdateTopupGroupRatioByJSONString(value)
	case "GitHubClientId":
		common.GitHubClientId = value
	case "GitHubClientSecret":
		common.GitHubClientSecret = value
	case "LinuxDOClientId":
		common.LinuxDOClientId = value
	case "LinuxDOClientSecret":
		common.LinuxDOClientSecret = value
	case "LinuxDOMinimumTrustLevel":
		common.LinuxDOMinimumTrustLevel, _ = strconv.Atoi(value)
	case "Footer":
		common.Footer = value
	case "SystemName":
		common.SystemName = value
	case "Logo":
		common.Logo = value
	case "WeChatServerAddress":
		common.WeChatServerAddress = value
	case "WeChatServerToken":
		common.WeChatServerToken = value
	case "WeChatAccountQRCodeImageURL":
		common.WeChatAccountQRCodeImageURL = value
	case "TelegramBotToken":
		common.TelegramBotToken = value
	case "TelegramBotName":
		common.TelegramBotName = value
	case "TurnstileSiteKey":
		common.TurnstileSiteKey = value
	case "TurnstileSecretKey":
		common.TurnstileSecretKey = value
	case "QuotaForNewUser":
		common.QuotaForNewUser, _ = strconv.Atoi(value)
	case "QuotaForInviter":
		common.QuotaForInviter, _ = strconv.Atoi(value)
	case "QuotaForInvitee":
		common.QuotaForInvitee, _ = strconv.Atoi(value)
	case "ReferralDepositPercent":
		percent, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
			percent = 0
		}
		common.SetReferralDepositPercent(percent)
	case "ReferralRequiredTopUpUSD":
		amount, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			return fmt.Errorf("referral required top-up USD must be greater than zero")
		}
		common.SetReferralRequiredTopUpUSD(amount)
	case "QuotaRemindThreshold":
		common.QuotaRemindThreshold, _ = strconv.Atoi(value)
	case "PreConsumedQuota":
		common.PreConsumedQuota, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitCount":
		setting.ModelRequestRateLimitCount = setting.ModelRequestRateLimitCountFromValue(value)
	case "ModelRequestRateLimitDurationMinutes":
		setting.ModelRequestRateLimitDurationMinutes, _ = strconv.Atoi(value)
	case setting.ModelRequestRateLimitDurationOption:
	case setting.ModelRequestRateLimitDurationActivatedOption:
	case "ModelRequestRateLimitSuccessCount":
		setting.ModelRequestRateLimitSuccessCount, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitGroup":
		err = setting.UpdateModelRequestRateLimitGroupByJSONString(value)
	case "RetryTimes":
		common.RetryTimes, _ = strconv.Atoi(value)
	case "DataExportInterval":
		common.DataExportInterval, _ = strconv.Atoi(value)
	case "DataExportDefaultTime":
		common.DataExportDefaultTime = value
	case "ModelRatio":
		err = ratio_setting.UpdateModelRatioByJSONString(value)
	case "GroupRatio":
		err = ratio_setting.UpdateGroupRatioByJSONString(value)
		if err == nil && normalizePricingReferences {
			err = NormalizePricingGroupReferences()
		}
	case "PricingGroups":
		err = ratio_setting.UpdatePricingGroupsByJSONString(value)
		if err == nil && normalizePricingReferences {
			err = NormalizePricingGroupReferences()
		}
	case "GroupGroupRatio":
		err = ratio_setting.UpdateGroupGroupRatioByJSONString(value)
	case "CompletionRatio":
		err = ratio_setting.UpdateCompletionRatioByJSONString(value)
	case "ModelPrice":
		err = ratio_setting.UpdateModelPriceByJSONString(value)
	case "CacheRatio":
		err = ratio_setting.UpdateCacheRatioByJSONString(value)
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(value)
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(value)
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(value)
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(value)
	case "TopUpLink":
		common.TopUpLink = value
	//case "ChatLink":
	//	common.ChatLink = value
	//case "ChatLink2":
	//	common.ChatLink2 = value
	case "ChannelDisableThreshold":
		common.ChannelDisableThreshold, _ = strconv.ParseFloat(value, 64)
	case "QuotaPerUnit":
		quotaPerUnit, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil {
			return parseErr
		}
		common.SetQuotaPerUnit(quotaPerUnit)
	case "SensitiveWords":
		setting.SensitiveWordsFromString(value)
	case "AutomaticDisableKeywords":
		operation_setting.AutomaticDisableKeywordsFromString(value)
	case "AutomaticDisableStatusCodes":
		err = operation_setting.AutomaticDisableStatusCodesFromString(value)
	case "AutomaticRetryStatusCodes":
		err = operation_setting.AutomaticRetryStatusCodesFromString(value)
	case "StreamCacheQueueLength":
		setting.StreamCacheQueueLength, _ = strconv.Atoi(value)
	case "PayMethods":
		err = operation_setting.UpdatePayMethodsByJsonString(value)
	case "WaffoPayMethods":
		// WaffoPayMethods is read directly from OptionMap via setting.GetWaffoPayMethods().
		// The value is already stored in OptionMap at the top of this function (line: common.OptionMap[key] = value).
		// No additional in-memory variable to update.
	}
	return err
}

// handleConfigUpdate 处理分层配置更新，返回是否已处理
func handleConfigUpdate(key, value string) bool {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return false // 不是分层配置
	}

	configName := parts[0]
	configKey := parts[1]

	// 获取配置对象
	cfg := config.GlobalConfig.Get(configName)
	if cfg == nil {
		return false // 未注册的配置
	}

	configMap := map[string]string{
		configKey: value,
	}
	if configName == "billing_setting" {
		_ = billing_setting.UpdateFromMap(configMap)
	} else {
		_ = config.UpdateConfigFromMap(cfg, configMap)
	}

	// 特定配置的后处理
	if configName == "performance_setting" {
		performance_setting.UpdateAndSync()
	} else if configName == "tool_price_setting" {
		operation_setting.RebuildToolPriceIndex()
	} else if configName == "billing_setting" {
		InvalidatePricingCache()
		ratio_setting.InvalidateExposedDataCache()
	}

	return true // 已处理
}
