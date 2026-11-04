package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestSupplierRequestIgnoresMetadataAndClientSchemas(t *testing.T) {
	for _, body := range []string{
		`{"metadata":{"type":"image","prompt_cache_retention":"24h"},"input":"hello"}`,
		`{"tools":[{"type":"function","name":"client_tool","parameters":{"type":"object","examples":[{"type":"image","cache_control":{"ttl":"12h"}}],"properties":{"kind":{"default":{"type":"code_interpreter"}}}}}]}`,
		`{"tools":[{"name":"client_tool","input_schema":{"type":"object","examples":[{"type":"audio","modalities":["audio"]}]}}]}`,
		`{"text":{"format":{"type":"json_schema","name":"result","schema":{"type":"object","examples":[{"type":"image","cache_control":{"ttl":"12h"}}]}}}}`,
		`{"tools":[{"functionDeclarations":[{"name":"client_tool","parameters":{"type":"object","examples":[{"inlineData":{"mimeType":"audio/mp3"}}]}}]}],"contents":[{"parts":[{"text":"hello"}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			ctx, info := supplierTestContext(body)
			price, err := modelPriceHelperSupplier(ctx, info, 10, &types.TokenCountMeta{MaxTokens: 10}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
				BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2), ReasoningUSD: supplierTestPrice(2),
			})
			require.NoError(t, err)
			require.NotNil(t, price.SupplierTariff)
		})
	}
}

func TestSupplierRequestStillRejectsActualUnpricedProtocolFeatures(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/input.png"}}]}]}`,
		`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.com/input.png"}]}]}`,
		`{"system":[{"type":"text","text":"cached instructions","cache_control":{"type":"ephemeral"}}]}`,
		`{"tools":[{"name":"client_tool","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]}`,
		`{"contents":[{"parts":[{"inlineData":{"mimeType":"audio/mp3","data":"a"}}]}]}`,
		`{"generationConfig":{"responseModalities":["AUDIO"]}}`,
		`{"item":{"type":"message","content":[{"type":"input_audio","audio":"a"}]}}`,
		`{"tools":[{"type":"code_interpreter"}]}`,
		`{"tools":[{"type":"web_search_preview"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			ctx, info := supplierTestContext(body)
			_, err := modelPriceHelperSupplier(ctx, info, 10, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
				BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2),
			})
			require.Error(t, err)
			require.False(t, info.AttemptDispatched)
		})
	}
}

func TestSupplierRealtimeIgnoresClientSchemaToolsExamples(t *testing.T) {
	_, info := supplierTestContext(`{}`)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken, AudioInputUSD: supplierTestPrice(1), AudioOutputUSD: supplierTestPrice(2)}
	require.NoError(t, ValidateSupplierRealtimeMessage(info, []byte(`{"type":"session.update","session":{"metadata":{"type":"image"},"tools":[{"type":"function","name":"client_tool","parameters":{"type":"object","examples":[{"tools":[{"type":"code_interpreter"}]}]}}]}}`)))
	require.Error(t, ValidateSupplierRealtimeMessage(info, []byte(`{"type":"session.update","session":{"tools":[{"type":"web_search_preview"}]}}`)))
}
