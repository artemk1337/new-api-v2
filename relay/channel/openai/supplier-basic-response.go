package openai

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

func projectSupplierOpenAIResponse(c *gin.Context, info *relaycommon.RelayInfo, data []byte, usage *dto.Usage) ([]byte, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		var response struct {
			Object string `json:"object"`
			Model  string `json:"model"`
			Data   []struct {
				Object    string          `json:"object"`
				Index     int             `json:"index"`
				Embedding json.RawMessage `json:"embedding"`
			} `json:"data"`
			Usage dto.Usage       `json:"usage"`
			Error json.RawMessage `json:"error,omitempty"`
		}
		if err := common.Unmarshal(data, &response); err != nil || supplierHasError(response.Error) {
			return nil, errSupplierResponse
		}
		response.Model = supplierPublicModel(c, info)
		response.Usage = supplierPublicUsage(*usage)
		return common.Marshal(response)
	case relayconstant.RelayModeModerations:
		var response struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Results []struct {
				Flagged    bool                `json:"flagged"`
				Categories map[string]bool     `json:"categories"`
				Scores     map[string]float64  `json:"category_scores"`
				InputTypes map[string][]string `json:"category_applied_input_types,omitempty"`
			} `json:"results"`
			Error json.RawMessage `json:"error,omitempty"`
		}
		if err := common.Unmarshal(data, &response); err != nil || supplierHasError(response.Error) {
			return nil, errSupplierResponse
		}
		response.ID = helper.GetResponseID(c)
		response.Model = supplierPublicModel(c, info)
		return common.Marshal(response)
	default:
		return projectSupplierChatResponse(c, info, data, false, usage)
	}
}
