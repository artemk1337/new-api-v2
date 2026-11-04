package service

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func priceQuotaPerUnit(info *relaycommon.RelayInfo) float64 {
	if info.PriceData.SupplierTariff != nil && info.PriceData.QuotaPerUnit > 0 {
		return info.PriceData.QuotaPerUnit
	}
	return common.GetQuotaPerUnit()
}

func supplierTieredResultError(info *relaycommon.RelayInfo, result billingexpr.TieredResult) error {
	if info.PriceData.SupplierTariff == nil {
		return nil
	}
	value := result.ActualQuotaBeforeGroup * info.TieredBillingSnapshot.GroupRatio
	if result.ActualQuotaBeforeGroup < 0 || result.ActualQuotaAfterGroup < 0 ||
		math.IsNaN(value) || math.IsInf(value, 0) || value >= float64(int(^uint(0)>>1)) {
		return fmt.Errorf("supplier billing exceeds the supported settlement range")
	}
	return nil
}
