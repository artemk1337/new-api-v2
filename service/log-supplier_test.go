package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTextLogSupplierIdentityUsesFrozenTariff(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("proxy_supplier_id", 22)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 11}}}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
	require.Equal(t, 11, other["admin_info"].(map[string]interface{})["proxy_supplier_id"])
	require.Equal(t, true, other["supplier_pricing"])
	info.PriceData.SupplierTariff = nil
	other = GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
	require.Equal(t, 0, other["admin_info"].(map[string]interface{})["proxy_supplier_id"], "a frozen unbound route must not inherit a later context")
}

func TestAsyncLogSupplierIdentitySurvivesPersistedTariff(t *testing.T) {
	task := model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
		SupplierID:    11,
		PriceSnapshot: &types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 11}},
	}}}
	encoded, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	var persisted model.TaskPrivateData
	require.NoError(t, common.Unmarshal(encoded, &persisted))
	task.PrivateData = persisted
	require.Zero(t, task.PrivateData.BillingContext.PriceSnapshot.SupplierTariff.SupplierID, "tariff identity is intentionally JSON-private")
	other := taskBillingOther(&task)
	require.Equal(t, 11, other["admin_info"].(map[string]interface{})["proxy_supplier_id"])
	task.PrivateData.BillingContext.PriceSnapshot.SupplierTariff.SupplierID = 33
	other = taskBillingOther(&task)
	require.Equal(t, 33, other["admin_info"].(map[string]interface{})["proxy_supplier_id"])
}
