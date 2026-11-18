package claude

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConfigureOpus55Reasoning applies the Messages contract for Opus 5.5.
func ConfigureOpus55Reasoning(request *dto.ClaudeRequest, effort, originalModel string) (bool, error) {
	const opus55 = "claude-opus-5-5"
	baseModel, suffixEffort, _ := reasoning.TrimEffortSuffix(request.Model)
	switch {
	case baseModel == opus55 && suffixEffort != "minimal":
		request.Model = baseModel
	case request.Model == opus55+"-thinking" && model_setting.GetClaudeSettings().ThinkingAdapterEnabled:
		suffixEffort = "high"
		if !model_setting.ShouldPreserveThinkingSuffix(originalModel) {
			request.Model = strings.TrimSuffix(request.Model, "-thinking")
		}
	default:
		return false, nil
	}

	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
	if request.Thinking != nil && request.Thinking.Type == "enabled" {
		request.Thinking.Type = "adaptive"
		request.Thinking.BudgetTokens = nil
	}
	if effort == "" {
		effort = suffixEffort
	}
	if effort == "" || gjson.GetBytes(request.OutputConfig, "effort").Exists() {
		return true, nil
	}

	var err error
	request.OutputConfig, err = sjson.SetBytes(request.OutputConfig, "effort", effort)
	return true, err
}
