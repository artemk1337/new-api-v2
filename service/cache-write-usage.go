package service

import "github.com/QuantumNous/new-api/dto"

// cacheWriteTokenCounts keeps explicit lifetime counters separate and returns
// the aggregate remainder without guessing its lifetime.
func cacheWriteTokenCounts(details dto.InputTokenDetails) (short, long, unknown int) {
	if details.CacheWriteTokensDetails != nil {
		short = max(details.CacheWriteTokensDetails.TTL30mTokens, 0)
		long = max(details.CacheWriteTokensDetails.TTL12hTokens, 0)
	}
	unknown = max(details.CacheWriteTokens-short-long, 0)
	return short, long, unknown
}
