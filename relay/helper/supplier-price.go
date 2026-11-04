package helper

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func supplierPriceError(info *relaycommon.RelayInfo) error {
	return newRoutePriceUnavailable(fmt.Sprintf("model %s price is unavailable for the selected group", info.OriginModelName))
}

func modelPriceHelperSupplier(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatio types.GroupRatioInfo, tariff types.SupplierModelTariff) (types.PriceData, error) {
	quotaPerUnit := common.GetQuotaPerUnit()
	for _, price := range tariff.Tools {
		if _, err := checkedSupplierQuota(price * quotaPerUnit * groupRatio.GroupRatio); err != nil {
			return types.PriceData{}, supplierPriceError(info)
		}
	}
	if err := ValidateSupplierRequest(c, info, tariff); err != nil {
		return types.PriceData{}, err
	}
	tariff = tariff.Clone()
	var price types.PriceData
	if tariff.BillingMode == types.SupplierBillingModeFixed {
		if tariff.PriceUSD == nil || (tariff.PriceUnit != "request" && tariff.PriceUnit != "per_second") {
			return price, supplierPriceError(info)
		}
		if tariff.PriceUnit == "per_second" && info.TaskRelayInfo == nil {
			return price, unsupportedSupplierFeature("request duration")
		}
		price = types.PriceData{
			ModelPrice: *tariff.PriceUSD, UsePrice: true, GroupRatioInfo: groupRatio,
		}
		quota, err := checkedSupplierQuota(*tariff.PriceUSD * quotaPerUnit * groupRatio.GroupRatio)
		if err != nil {
			return types.PriceData{}, supplierPriceError(info)
		}
		price.Quota = quota
		price.QuotaToPreConsume = price.Quota
		price.FreeModel = price.Quota == 0
		info.TieredBillingSnapshot = nil
	} else {
		expression, err := SupplierTariffExpression(tariff)
		if err != nil {
			return price, supplierPriceError(info)
		}
		price, err = modelPriceHelperExpression(c, info, promptTokens, meta, groupRatio, expression)
		if err != nil {
			return types.PriceData{}, supplierPriceError(info)
		}
		if _, err := checkedSupplierQuota(info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup * groupRatio.GroupRatio); err != nil {
			return types.PriceData{}, supplierPriceError(info)
		}
		if tariff.InputUSD != nil {
			price.ModelRatio = *tariff.InputUSD / 2
			if *tariff.InputUSD > 0 {
				price.CompletionRatio = supplierRatio(tariff.OutputUSD, *tariff.InputUSD)
				price.CacheRatio = supplierRatio(tariff.CacheReadUSD, *tariff.InputUSD)
				price.CacheCreationRatio = supplierRatio(tariff.CacheWriteUSD, *tariff.InputUSD)
				price.CacheCreation5mRatio = price.CacheCreationRatio
				price.CacheCreation1hRatio = supplierRatio(tariff.CacheWrite1hUSD, *tariff.InputUSD)
				price.ImageRatio = supplierRatio(tariff.ImageUSD, *tariff.InputUSD)
				price.AudioRatio = supplierRatio(tariff.AudioInputUSD, *tariff.InputUSD)
			}
		}
		if tariff.AudioInputUSD != nil && *tariff.AudioInputUSD > 0 {
			price.AudioCompletionRatio = supplierRatio(tariff.AudioOutputUSD, *tariff.AudioInputUSD)
		}
	}
	price.SupplierTariff = &tariff
	price.QuotaPerUnit = quotaPerUnit
	if info.TieredBillingSnapshot != nil {
		price.QuotaPerUnit = info.TieredBillingSnapshot.QuotaPerUnit
	}
	info.PriceData = price
	return price, nil
}

func supplierRatio(value *float64, base float64) float64 {
	if value == nil {
		return 0
	}
	ratio := *value / base
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0
	}
	return ratio
}

func ApplySupplierTaskPrice(price *types.PriceData, ratios map[string]float64) error {
	tariff := price.SupplierTariff
	units := 1.0
	var otherRatios map[string]float64
	if tariff.PriceUnit == "per_second" {
		units = ratios["seconds"]
		if units <= 0 {
			return fmt.Errorf("task duration is required for pricing")
		}
		otherRatios = map[string]float64{"seconds": units}
	}
	quotaPerUnit := price.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = common.GetQuotaPerUnit()
	}
	quota, err := checkedSupplierQuota(*tariff.PriceUSD * units * price.GroupRatioInfo.GroupRatio * quotaPerUnit)
	if err != nil {
		return err
	}
	price.OtherRatios = otherRatios
	price.Quota = quota
	price.QuotaToPreConsume = price.Quota
	return nil
}

func checkedSupplierQuota(value float64) (int, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value >= float64(int(^uint(0)>>1)) {
		return 0, fmt.Errorf("supplier price exceeds the supported billing range")
	}
	return billingexpr.QuotaRound(value), nil
}

// SupplierTariffExpression keeps token normalization and settlement on the
// existing expression engine, including tariffs with a free input lane.
func SupplierTariffExpression(tariff types.SupplierModelTariff) (string, error) {
	if tariff.BillingMode == types.SupplierBillingModeExpression {
		if strings.TrimSpace(tariff.Expression) == "" {
			return "", fmt.Errorf("missing billing expression")
		}
		if err := billingexpr.ValidateSupplierExpression(tariff.Expression); err != nil {
			return "", err
		}
		return tariff.Expression, nil
	}
	if tariff.BillingMode != types.SupplierBillingModeToken || tariff.InputUSD == nil || tariff.OutputUSD == nil {
		return "", fmt.Errorf("missing token price")
	}
	lanes := []struct {
		name  string
		price *float64
	}{
		{"p", tariff.InputUSD}, {"c", tariff.OutputUSD},
		{"cr", tariff.CacheReadUSD}, {"cc", tariff.CacheWriteUSD},
		{"cc1h", tariff.CacheWrite1hUSD}, {"img", tariff.ImageUSD},
		{"img_o", tariff.ImageOutputUSD}, {"ai", tariff.AudioInputUSD},
		{"ao", tariff.AudioOutputUSD}, {"rt", tariff.ReasoningUSD},
	}
	terms := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		price := 0.0
		if lane.price != nil {
			price = *lane.price
		}
		// An unknown lane is excluded from p/c. Its unexpected use is handled
		// as an incomplete settlement, never priced with default model rates.
		terms = append(terms, fmt.Sprintf("%s * %.17g", lane.name, price))
	}
	return strings.Join(terms, " + "), nil
}

func supplierLanePriced(tariff types.SupplierModelTariff, variable string, price *float64) bool {
	return tariff.BillingMode == types.SupplierBillingModeFixed || price != nil ||
		tariff.BillingMode == types.SupplierBillingModeExpression && billingexpr.UsedVars(tariff.Expression)[variable]
}

func unsupportedSupplierFeature(feature string) error {
	return newRoutePriceUnavailable(fmt.Sprintf("the selected group has no price for %s", feature))
}

func SupplierImageToolPrice(tariff types.SupplierModelTariff, quality, size string) (float64, bool) {
	if price, ok := tariff.Tools["image_generation:"+quality+":"+size]; ok {
		return price, true
	}
	price, ok := tariff.Tools["image_generation"]
	return price, ok
}

// ValidateSupplierRequest rejects advertised, unpriced features before a
// provider can accept the request. Function tools do not carry a provider fee.
func ValidateSupplierRequest(c *gin.Context, info *relaycommon.RelayInfo, tariff types.SupplierModelTariff) error {
	path := info.RequestURLPath
	if path == "" && c != nil && c.Request != nil {
		path = c.Request.URL.Path
	}
	if tariff.BillingMode == types.SupplierBillingModeFixed && (info.RelayFormat == types.RelayFormatOpenAIImage || strings.Contains(path, "/images/")) {
		return unsupportedSupplierFeature("fixed image variants and quantities")
	}
	input, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return err
	}
	if len(input.Body) == 0 && info.Request != nil {
		input, err = BuildBillingExprRequestInputFromRequest(info.Request, info.RequestHeaders)
		if err != nil {
			return err
		}
	}
	info.BillingRequestInput = &input
	var body any
	if len(input.Body) > 0 {
		if err := common.Unmarshal(input.Body, &body); err != nil {
			return fmt.Errorf("invalid pricing request")
		}
	}
	if request, ok := body.(map[string]any); ok {
		if err := validateSupplierProtocolRequest(request, tariff); err != nil {
			return err
		}
	}
	if strings.Contains(path, "/realtime") && (!supplierLanePriced(tariff, "ai", tariff.AudioInputUSD) || !supplierLanePriced(tariff, "ao", tariff.AudioOutputUSD)) {
		return unsupportedSupplierFeature("realtime audio")
	}
	if strings.Contains(path, "/images/") && !supplierLanePriced(tariff, "img_o", tariff.ImageOutputUSD) {
		return unsupportedSupplierFeature("image output")
	}
	if strings.Contains(path, "/audio/speech") && !supplierLanePriced(tariff, "ao", tariff.AudioOutputUSD) {
		return unsupportedSupplierFeature("audio output")
	}
	if (strings.Contains(path, "/audio/transcriptions") || strings.Contains(path, "/audio/translations")) && !supplierLanePriced(tariff, "ai", tariff.AudioInputUSD) {
		return unsupportedSupplierFeature("audio input")
	}
	return nil
}

func ValidateSupplierRealtimeMessage(info *relaycommon.RelayInfo, body []byte) error {
	tariff := info.PriceData.SupplierTariff
	if tariff == nil {
		return nil
	}
	candidate := *info
	candidate.Request = nil
	candidate.BillingRequestInput = &billingexpr.RequestInput{Body: append([]byte(nil), body...)}
	if err := ValidateSupplierRequest(nil, &candidate, *tariff); err != nil {
		return err
	}
	var message map[string]any
	if err := common.Unmarshal(body, &message); err != nil {
		return fmt.Errorf("invalid realtime pricing request")
	}
	return validateSupplierRealtimeToolDefinitions(message)
}

func ValidateSupplierUsage(info *relaycommon.RelayInfo, usage *dto.Usage) error {
	tariff := info.PriceData.SupplierTariff
	if tariff == nil || usage == nil || tariff.BillingMode == types.SupplierBillingModeFixed {
		return nil
	}
	lanes := []struct {
		count    int
		variable string
		price    *float64
	}{
		{usage.PromptTokensDetails.CachedTokens, "cr", tariff.CacheReadUSD},
		{max(usage.ClaudeCacheCreation5mTokens, usage.PromptTokensDetails.CachedCreationTokens-usage.ClaudeCacheCreation1hTokens), "cc", tariff.CacheWriteUSD},
		{usage.ClaudeCacheCreation1hTokens, "cc1h", tariff.CacheWrite1hUSD},
		{usage.PromptTokensDetails.ImageTokens, "img", tariff.ImageUSD},
		{usage.PromptTokensDetails.AudioTokens, "ai", tariff.AudioInputUSD},
		{usage.CompletionTokenDetails.AudioTokens, "ao", tariff.AudioOutputUSD},
		{usage.CompletionTokenDetails.ImageTokens, "img_o", tariff.ImageOutputUSD},
		{usage.CompletionTokenDetails.ReasoningTokens, "rt", tariff.ReasoningUSD},
	}
	for _, lane := range lanes {
		if lane.count > 0 && !supplierLanePriced(*tariff, lane.variable, lane.price) {
			return unsupportedSupplierFeature(lane.variable)
		}
	}
	return nil
}

func ValidateSupplierRealtimeUsage(info *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if usage == nil {
		return nil
	}
	return ValidateSupplierUsage(info, &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: usage.InputTokenDetails.CachedTokens,
			AudioTokens:  usage.InputTokenDetails.AudioTokens,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{
			AudioTokens:     usage.OutputTokenDetails.AudioTokens,
			ReasoningTokens: usage.OutputTokenDetails.ReasoningTokens,
		},
	})
}
