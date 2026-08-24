package common

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuotaPerUnitConcurrentAccess(t *testing.T) {
	original := GetQuotaPerUnit()
	t.Cleanup(func() { SetQuotaPerUnit(original) })

	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		wg.Add(1)
		go func(value float64) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				SetQuotaPerUnit(value)
				_ = GetQuotaPerUnit()
			}
		}(float64(i))
	}
	wg.Wait()
	require.Greater(t, GetQuotaPerUnit(), float64(0))
}
