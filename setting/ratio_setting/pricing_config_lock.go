package ratio_setting

import "sync"

// pricingConfigMutex keeps a pricing calculation from observing a partially
// applied group of model-pricing option maps.
var pricingConfigMutex sync.RWMutex

func LockPricingConfigRead() func() {
	pricingConfigMutex.RLock()
	return pricingConfigMutex.RUnlock
}

func LockPricingConfigWrite() func() {
	pricingConfigMutex.Lock()
	return pricingConfigMutex.Unlock
}
