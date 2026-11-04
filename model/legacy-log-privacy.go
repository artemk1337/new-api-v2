package model

func historicalSupplierMetadata(other map[string]interface{}) bool {
	if supplier, _ := other["supplier_pricing"].(bool); supplier {
		return true
	}
	for _, key := range []string{"proxy_supplier_id", "supplier_id", "supplier_profile_id", "profile_id"} {
		switch value := other[key].(type) {
		case float64:
			if value != 0 {
				return true
			}
		case int:
			if value != 0 {
				return true
			}
		}
	}
	for _, key := range []string{"SupplierTariff", "supplier_tariff"} {
		if value, exists := other[key]; exists && value != nil {
			return true
		}
	}
	for _, key := range []string{"admin_info", "billing_context", "price_snapshot", "PriceSnapshot", "billing_snapshot"} {
		if nested, ok := other[key].(map[string]interface{}); ok && historicalSupplierMetadata(nested) {
			return true
		}
	}
	return false
}

// Public history must not expose previously stored procurement contracts after
// the supplier feature is removed. Keep only known public scalar usage fields.
func publicHistoricalSupplierLogFields(other map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, key := range []string{
		"requested_model", "request_path", "frt", "reasoning_effort", "ws", "audio", "claude", "image",
		"audio_input", "audio_output", "text_input", "text_output", "cache_tokens", "cache_creation_tokens",
		"cache_creation_tokens_5m", "cache_creation_tokens_1h", "image_output", "web_search_call_count", "file_search_call_count",
		"audio_input_token_count", "image_generation_call", "billing_source", "billing_preference", "billing_overage_source",
		"billing_overage_quota", "subscription_id", "subscription_pre_consumed", "subscription_post_delta", "subscription_plan_id",
		"subscription_plan_title", "subscription_total", "subscription_used", "subscription_remain", "subscription_consumed",
		"wallet_quota_deducted", "reserved_quota", "released_quota", "charged_on_error",
	} {
		switch value := other[key].(type) {
		case string:
			result[key] = value
		case bool:
			result[key] = value
		case float64:
			result[key] = value
		}
	}
	return result
}
