package model

import (
	"errors"
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
	"ModelRatio":                   {},
	"ModelPrice":                   {},
	"CompletionRatio":              {},
	"CacheRatio":                   {},
	"CreateCacheRatio":             {},
	"ImageRatio":                   {},
	"AudioRatio":                   {},
	"AudioCompletionRatio":         {},
	"billing_setting.billing_mode": {},
	"billing_setting.billing_expr": {},
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
	common.OptionMap["YooKassaEnabled"] = strconv.FormatBool(setting.YooKassaEnabled)
	common.OptionMap["YooKassaShopID"] = setting.YooKassaShopID
	common.OptionMap["YooKassaSecretKey"] = setting.YooKassaSecretKey
	common.OptionMap["YooKassaReturnURL"] = setting.YooKassaReturnURL
	common.OptionMap["YooKassaPaymentMethods"] = setting.YooKassaPaymentMethods
	common.OptionMap["NOWPaymentsEnabled"] = strconv.FormatBool(setting.NOWPaymentsEnabled)
	common.OptionMap["NOWPaymentsAPIKey"] = setting.NOWPaymentsAPIKey
	common.OptionMap["NOWPaymentsIPNSecret"] = setting.NOWPaymentsIPNSecret
	common.OptionMap["NOWPaymentsPriceCurrency"] = setting.NOWPaymentsPriceCurrency
	common.OptionMap["NOWPaymentsPayCurrency"] = setting.NOWPaymentsPayCurrency
	common.OptionMap["NOWPaymentsIPNCallbackURL"] = setting.NOWPaymentsIPNCallbackURL
	common.OptionMap["TopupGroupRatio"] = common.TopupGroupRatio2JSONString()
	common.OptionMap["PricingGroups"] = "[]"
	common.OptionMap["AutoGroups"] = setting.AutoGroups2JsonString()
	common.OptionMap["DefaultUseAutoGroup"] = strconv.FormatBool(setting.DefaultUseAutoGroup)
	common.OptionMap["PayMethods"] = operation_setting.PayMethods2JsonString()
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
	common.OptionMap["QuotaPerUnit"] = strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64)
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
	for _, option := range options {
		switch option.Key {
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
		if option.Key == removedChatsOptionKey {
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

	if key == removedChatsOptionKey {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, key)
		common.OptionMapRWMutex.Unlock()
		return nil
	}
	if key == "UserUsableGroups" {
		return nil
	}
	if err := validateOptionValue(key, value); err != nil {
		return err
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
	if err := persistOptionsAndRuntime(values); err != nil {
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
	case "payment_setting.amount_cashback":
		var cashbacks operation_setting.AmountCashbackConfig
		if err := common.Unmarshal([]byte(value), &cashbacks); err != nil {
			return err
		}
		return operation_setting.ValidateAmountCashback(cashbacks)
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
	default:
		return nil
	}
}

func normalizeOptionValueForSave(key string, value string) (string, error) {
	var (
		normalized string
		ok         bool
		err        error
	)
	switch key {
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
	return updateOptionsBulkLocked(values)
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

func updateOptionsBulkLocked(values map[string]string) error {
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
	if err := validateModelRequestRateLimitDurationActivation(normalizedValues); err != nil {
		return err
	}
	if normalizedValues[setting.ModelRequestRateLimitDurationActivatedOption] == "true" && !activationWasEnabled {
		normalizedValues[setting.ModelRequestRateLimitDurationActivationAtOption] = strconv.FormatInt(modelRequestRateLimitActivationNow()+modelRequestRateLimitActivationDelay(), 10)
	}
	if err := persistOptionsAndRuntime(normalizedValues); err != nil {
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

func persistOptionsAndRuntime(values map[string]string) error {
	keys := sortedOptionKeys(values)
	pricingNormalizer, pricingGroupsChanged, err := pricingGroupNormalizerForOptions(keys, values)
	if err != nil {
		return err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
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
	if pricingGroupsChanged {
		InvalidatePricingCache()
	}
	return nil
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
		setting.NOWPaymentsPriceCurrency = value
	case "NOWPaymentsPayCurrency":
		setting.NOWPaymentsPayCurrency = value
	case "NOWPaymentsIPNCallbackURL":
		setting.NOWPaymentsIPNCallbackURL = value
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
		common.QuotaPerUnit, _ = strconv.ParseFloat(value, 64)
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
