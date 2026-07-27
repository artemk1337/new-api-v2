package billing_setting

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUpdateFromMapSynchronizesReaders(t *testing.T) {
	originalModes := GetBillingModeCopy()
	originalExprs := GetBillingExprCopy()
	originalModesJSON, err := common.Marshal(originalModes)
	require.NoError(t, err)
	originalExprsJSON, err := common.Marshal(originalExprs)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, UpdateFromMap(map[string]string{
			BillingModeField: string(originalModesJSON),
			BillingExprField: string(originalExprsJSON),
		}))
	})

	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = GetBillingMode("model-a")
					_, _ = GetBillingExpr("model-a")
					_ = GetPricingSyncData(map[string]any{})
				}
			}
		}()
	}

	for range 100 {
		require.NoError(t, UpdateFromMap(map[string]string{
			BillingModeField: `{"model-a":"tiered_expr"}`,
			BillingExprField: `{"model-a":"tier(\"default\", p)"}`,
		}))
		require.NoError(t, UpdateFromMap(map[string]string{
			BillingModeField: `{}`,
			BillingExprField: `{}`,
		}))
	}
	close(done)
	readers.Wait()
}
