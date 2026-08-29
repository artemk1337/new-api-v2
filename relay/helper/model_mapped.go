package helper

import (
	"errors"
	"fmt"
	"strings"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
	if err := ValidateModelMappingComposition(
		basecommon.GetContextKeyString(c, constant.ContextKeyTokenModelMapping),
		c.GetString("model_mapping"),
		basecommon.GetContextKeyString(c, constant.ContextKeyRequestedModel),
		info.RelayMode,
	); err != nil {
		return err
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

// ValidateModelMappingComposition rejects cycles created by applying a key
// mapping first and a channel mapping afterwards. Each mapping is valid on its
// own in this case, but their composition would route a model back to a model
// already seen in the key mapping.
func ValidateModelMappingComposition(keyMapping, channelMapping, modelName string, relayMode int) error {
	if modelName == "" {
		return nil
	}
	if relayMode == relayconstant.RelayModeResponsesCompact && strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		modelName = strings.TrimSuffix(modelName, ratio_setting.CompactModelSuffix)
	}
	visited := map[string]bool{modelName: true}
	currentModel, err := followModelMapping(keyMapping, modelName, visited)
	if err != nil {
		return err
	}
	_, err = followModelMapping(channelMapping, currentModel, visited)
	return err
}

// ValidateModelMappingCandidates applies the composition check to every
// channel that may be selected after pre-consume.
func ValidateModelMappingCandidates(keyMapping, modelName string, relayMode int, channelMappings []string) error {
	for _, channelMapping := range channelMappings {
		if err := ValidateModelMappingComposition(keyMapping, channelMapping, modelName, relayMode); err != nil {
			return err
		}
	}
	return nil
}

func followModelMapping(modelMapping, modelName string, visited map[string]bool) (string, error) {
	if modelMapping == "" || modelMapping == "{}" {
		return modelName, nil
	}

	modelMap := make(map[string]string)
	if err := basecommon.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
		return "", fmt.Errorf("unmarshal_model_mapping_failed")
	}
	currentModel := modelName
	for {
		mappedModel, exists := modelMap[currentModel]
		if !exists || mappedModel == "" || mappedModel == currentModel {
			return currentModel, nil
		}
		if visited[mappedModel] {
			return "", errors.New("model_mapping_contains_cycle")
		}
		visited[mappedModel] = true
		currentModel = mappedModel
	}
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
