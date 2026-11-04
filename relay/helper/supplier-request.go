package helper

import (
	"strings"

	"github.com/QuantumNous/new-api/types"
)

// Inspect protocol fields only. Metadata, function schemas, arguments and
// examples may contain arbitrary JSON that does not request a paid feature.
func validateSupplierProtocolRequest(request map[string]any, tariff types.SupplierModelTariff) error {
	if err := validateSupplierRequestOptions(request, tariff); err != nil {
		return err
	}
	if tools, ok := request["tools"].([]any); ok {
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				return unsupportedSupplierFeature("this tool")
			}
			kind, _ := tool["type"].(string)
			switch {
			case kind == "function":
			case kind == "" && (tool["input_schema"] != nil || tool["functionDeclarations"] != nil || tool["function_declarations"] != nil):
			case kind == "file_search":
				if _, priced := tariff.Tools["file_search"]; !priced {
					return unsupportedSupplierFeature("file search")
				}
			case strings.HasPrefix(kind, "web_search"):
				key := "web_search_preview"
				if strings.HasPrefix(kind, "web_search_") && kind != "web_search_preview" {
					key = "web_search"
				}
				if _, priced := tariff.Tools[key]; !priced {
					return unsupportedSupplierFeature("web search")
				}
			default:
				return unsupportedSupplierFeature("this tool")
			}
			if err := validateSupplierRequestOptions(tool, tariff); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"messages", "system", "input", "contents", "systemInstruction", "system_instruction", "item"} {
		if err := validateSupplierInputContent(request[key], tariff); err != nil {
			return err
		}
	}
	for _, key := range []string{"session", "response"} {
		if options, ok := request[key].(map[string]any); ok {
			if err := validateSupplierProtocolRequest(options, tariff); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"generationConfig", "generation_config"} {
		if options, ok := request[key].(map[string]any); ok {
			if err := validateSupplierRequestOptions(options, tariff); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSupplierInputContent(value any, tariff types.SupplierModelTariff) error {
	switch item := value.(type) {
	case []any:
		for _, part := range item {
			if err := validateSupplierInputContent(part, tariff); err != nil {
				return err
			}
		}
	case map[string]any:
		if err := validateSupplierRequestOptions(item, tariff); err != nil {
			return err
		}
		kind, _ := item["type"].(string)
		switch kind {
		case "image_url", "input_image", "image":
			if !supplierLanePriced(tariff, "img", tariff.ImageUSD) {
				return unsupportedSupplierFeature("image input")
			}
		case "input_audio", "audio":
			if !supplierLanePriced(tariff, "ai", tariff.AudioInputUSD) {
				return unsupportedSupplierFeature("audio input")
			}
		}
		for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data"} {
			if part, ok := item[key].(map[string]any); ok {
				mime, _ := part["mimeType"].(string)
				if mime == "" {
					mime, _ = part["mime_type"].(string)
				}
				if strings.HasPrefix(mime, "image/") && !supplierLanePriced(tariff, "img", tariff.ImageUSD) {
					return unsupportedSupplierFeature("image input")
				}
				if strings.HasPrefix(mime, "audio/") && !supplierLanePriced(tariff, "ai", tariff.AudioInputUSD) {
					return unsupportedSupplierFeature("audio input")
				}
			}
		}
		for _, key := range []string{"content", "parts"} {
			if err := validateSupplierInputContent(item[key], tariff); err != nil {
				return err
			}
		}
		if kind == "function_call_output" {
			if output, ok := item["output"].([]any); ok {
				return validateSupplierInputContent(output, tariff)
			}
		}
	}
	return nil
}

func validateSupplierRequestOptions(options map[string]any, tariff types.SupplierModelTariff) error {
	if cache, ok := options["cache_control"].(map[string]any); ok {
		lane, price := "cc", tariff.CacheWriteUSD
		if scope, ok := cache["scope"]; ok && scope != nil && scope != "" {
			return unsupportedSupplierFeature("cache scope")
		}
		if ttl, ok := cache["ttl"]; ok && ttl != "" && ttl != "5m" && ttl != "1h" {
			return unsupportedSupplierFeature("cache retention")
		}
		if cache["ttl"] == "1h" {
			lane, price = "cc1h", tariff.CacheWrite1hUSD
		}
		if !supplierLanePriced(tariff, lane, price) {
			return unsupportedSupplierFeature("cache creation")
		}
	}
	if retention, ok := options["prompt_cache_retention"]; ok && retention != nil && retention != "in-memory" {
		return unsupportedSupplierFeature("cache retention")
	}
	if options["cachedContent"] != nil || options["cached_content"] != nil {
		if !supplierLanePriced(tariff, "cr", tariff.CacheReadUSD) {
			return unsupportedSupplierFeature("cache read")
		}
	}
	for _, key := range []string{"modalities", "responseModalities", "response_modalities"} {
		if modalities, ok := options[key].([]any); ok {
			for _, raw := range modalities {
				if modality, ok := raw.(string); ok && strings.EqualFold(modality, "audio") && !supplierLanePriced(tariff, "ao", tariff.AudioOutputUSD) {
					return unsupportedSupplierFeature("audio output")
				}
			}
		}
	}
	return nil
}

func validateSupplierRealtimeToolDefinitions(request map[string]any) error {
	if tools, ok := request["tools"].([]any); ok {
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || tool["type"] != "function" {
				return unsupportedSupplierFeature("realtime server tools")
			}
		}
	}
	for _, key := range []string{"session", "response"} {
		if options, ok := request[key].(map[string]any); ok {
			if err := validateSupplierRealtimeToolDefinitions(options); err != nil {
				return err
			}
		}
	}
	return nil
}
