package helper

import (
	"errors"
	"fmt"
	"strings"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *common.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &common.ChannelMeta{}
	}

	isResponsesCompact := info.RelayMode == relayconstant.RelayModeResponsesCompact
	originModelName := info.OriginModelName
	mappingModelName := originModelName
	if isResponsesCompact && strings.HasSuffix(originModelName, ratio_setting.CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, ratio_setting.CompactModelSuffix)
	}

	targetModelName, isMapped, err := ResolveModelMapping(c.GetString("model_mapping"), mappingModelName)
	if err != nil {
		return err
	}
	info.IsModelMapped = isMapped
	info.BillingModelName = ""
	if isMapped {
		info.UpstreamModelName = targetModelName
		info.BillingModelName = targetModelName
		if isResponsesCompact {
			info.BillingModelName = ratio_setting.WithCompactModelSuffix(targetModelName)
		}
	}

	if isResponsesCompact {
		finalUpstreamModelName := mappingModelName
		if info.IsModelMapped && info.UpstreamModelName != "" {
			finalUpstreamModelName = info.UpstreamModelName
		}
		info.UpstreamModelName = finalUpstreamModelName
		info.SentUpstreamModelName = info.UpstreamModelName
		info.OriginModelName = ratio_setting.WithCompactModelSuffix(finalUpstreamModelName)
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}

func ResolveRelayModelMapping(modelMapping, originModelName string, relayMode int) (string, bool, error) {
	mappingModelName := originModelName
	if relayMode == relayconstant.RelayModeResponsesCompact && strings.HasSuffix(originModelName, ratio_setting.CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, ratio_setting.CompactModelSuffix)
	}
	targetModelName, isMapped, err := ResolveModelMapping(modelMapping, mappingModelName)
	if err != nil || !isMapped {
		return targetModelName, isMapped, err
	}
	if relayMode == relayconstant.RelayModeResponsesCompact {
		return ratio_setting.WithCompactModelSuffix(targetModelName), true, nil
	}
	return targetModelName, true, nil
}

// ResolveCandidateModelMapping accepts a target only when every eligible
// channel resolves the requested model identically. This keeps pre-consume
// from reserving against one provider target and dispatching another on retry.
func ResolveCandidateModelMapping(modelMappings []string, originModelName string, relayMode int) (string, bool, error) {
	targets := make(map[string]struct{}, len(modelMappings))
	for _, modelMapping := range modelMappings {
		target, isMapped, err := ResolveRelayModelMapping(modelMapping, originModelName, relayMode)
		if err != nil {
			return "", false, err
		}
		if !isMapped {
			target = originModelName
		}
		targets[target] = struct{}{}
	}
	if len(targets) == 0 {
		return originModelName, false, nil
	}
	if len(targets) != 1 {
		return "", false, errors.New("model_mapping_has_conflicting_targets")
	}
	for target := range targets {
		return target, target != originModelName, nil
	}
	return originModelName, false, nil
}

// ResolveModelMapping returns the terminal mapped model without mutating relay
// state. It is used before pre-consume, when the initially selected channel is
// available in context but RelayInfo must not acquire ChannelMeta yet.
func ResolveModelMapping(modelMapping, modelName string) (string, bool, error) {
	if modelMapping == "" || modelMapping == "{}" {
		return modelName, false, nil
	}

	modelMap := make(map[string]string)
	if err := basecommon.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
		return "", false, fmt.Errorf("unmarshal_model_mapping_failed")
	}

	currentModel := modelName
	visitedModels := map[string]bool{currentModel: true}
	for {
		mappedModel, exists := modelMap[currentModel]
		if !exists || mappedModel == "" {
			return currentModel, currentModel != modelName, nil
		}
		if visitedModels[mappedModel] {
			if mappedModel == currentModel && currentModel != modelName {
				return currentModel, true, nil
			}
			if mappedModel == modelName && currentModel == modelName {
				return modelName, false, nil
			}
			return "", false, errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}
}
