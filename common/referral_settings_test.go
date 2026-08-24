package common

import (
	"sync"
	"testing"
)

func TestReferralDepositPercentConcurrentAccess(t *testing.T) {
	original := GetReferralDepositPercent()
	t.Cleanup(func() { SetReferralDepositPercent(original) })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(value float64) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				SetReferralDepositPercent(value)
				_ = GetReferralDepositPercent()
			}
		}(float64(i))
	}
	wg.Wait()
}
