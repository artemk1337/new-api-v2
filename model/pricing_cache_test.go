package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPricingCacheConcurrentInvalidation(t *testing.T) {
	truncateTables(t)
	InvalidatePricingCache()
	t.Cleanup(InvalidatePricingCache)

	require.NotNil(t, GetPricing())

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start
		for range 50 {
			_ = GetPricing()
			_ = GetVendors()
			_ = GetSupportedEndpointMap()
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range 50 {
			InvalidatePricingCache()
			_ = GetPricing()
		}
	}()

	close(start)
	workers.Wait()
}
