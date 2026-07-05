package perfmetrics

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearHotBucketsForTest() {
	hotBuckets.Range(func(key, _ any) bool {
		hotBuckets.Delete(key)
		return true
	})
}

func TestRecordStoresPricingGroupID(t *testing.T) {
	clearHotBucketsForTest()
	t.Cleanup(clearHotBucketsForTest)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true}
	]`))

	Record(Sample{
		Model:     "gpt-perf",
		Group:     "Renamed VIP",
		LatencyMs: 100,
		Success:   true,
	})
	Record(Sample{
		Model:     "gpt-perf",
		Group:     "paid-users",
		LatencyMs: 100,
		Success:   true,
	})

	groups := make([]string, 0, 2)
	hotBuckets.Range(func(key, _ any) bool {
		bucket := key.(bucketKey)
		if bucket.model == "gpt-perf" {
			groups = append(groups, bucket.group)
		}
		return true
	})

	assert.ElementsMatch(t, []string{"1", "2"}, groups)
}
