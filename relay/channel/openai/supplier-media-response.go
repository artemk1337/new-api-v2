package openai

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func projectSupplierImageResponse(data []byte) ([]byte, error) {
	var response struct {
		Created           int64           `json:"created,omitempty"`
		CreatedAt         int64           `json:"created_at,omitempty"`
		Type              string          `json:"type,omitempty"`
		Data              []dto.ImageData `json:"data,omitempty"`
		B64JSON           string          `json:"b64_json,omitempty"`
		PartialImageB64   string          `json:"partial_image_b64,omitempty"`
		PartialImageIndex *int            `json:"partial_image_index,omitempty"`
		URL               string          `json:"url,omitempty"`
		RevisedPrompt     string          `json:"revised_prompt,omitempty"`
		Background        string          `json:"background,omitempty"`
		OutputFormat      string          `json:"output_format,omitempty"`
		Size              string          `json:"size,omitempty"`
		Quality           string          `json:"quality,omitempty"`
		Usage             *dto.Usage      `json:"usage,omitempty"`
		Error             json.RawMessage `json:"error,omitempty"`
	}
	if err := common.Unmarshal(data, &response); err != nil || supplierHasError(response.Error) || response.Type == "error" || response.Type == "upstream_error" {
		return nil, errSupplierResponse
	}
	if response.Usage != nil {
		usage := supplierPublicUsage(*response.Usage)
		response.Usage = &usage
	}
	return common.Marshal(response)
}

func projectSupplierAudioResponse(data []byte) ([]byte, error) {
	var response struct {
		Type     string        `json:"type,omitempty"`
		Task     string        `json:"task,omitempty"`
		Text     *string       `json:"text,omitempty"`
		Language string        `json:"language,omitempty"`
		Duration *float64      `json:"duration,omitempty"`
		Segments []dto.Segment `json:"segments,omitempty"`
		Words    []struct {
			Word  string  `json:"word"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"words,omitempty"`
		Audio             *string         `json:"audio,omitempty"`
		Delta             *string         `json:"delta,omitempty"`
		Data              string          `json:"data,omitempty"`
		Format            string          `json:"format,omitempty"`
		Transcript        *string         `json:"transcript,omitempty"`
		SequenceNumber    *int            `json:"sequence_number,omitempty"`
		InputTextTokens   *int            `json:"input_text_tokens,omitempty"`
		OutputAudioTokens *int            `json:"output_audio_tokens,omitempty"`
		Usage             *dto.Usage      `json:"usage,omitempty"`
		Error             json.RawMessage `json:"error,omitempty"`
	}
	if err := common.Unmarshal(data, &response); err != nil || supplierHasError(response.Error) || response.Type == "error" || response.Type == "upstream_error" {
		return nil, errSupplierResponse
	}
	if response.Usage != nil {
		usage := supplierPublicUsage(*response.Usage)
		response.Usage = &usage
	}
	return common.Marshal(response)
}
