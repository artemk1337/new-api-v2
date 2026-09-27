package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestValidateDirectUSDTOptionValuesIncludesWalletPool(t *testing.T) {
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousWallets := setting.USDTReceivingWallets
	t.Cleanup(func() {
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		setting.USDTReceivingWallets = previousWallets
	})

	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = ""
	setting.USDTTRC20APIKey = ""
	setting.USDTReceivingWallets = ""

	require.Error(t, validateDirectUSDTOptionValues(map[string]string{
		"USDTReceivingWallets": "",
	}))
	require.Error(t, validateDirectUSDTOptionValues(map[string]string{
		"USDTReceivingWallets": `[{
			"network":"SOLANA","address":"placeholder","enabled":false
		}]`,
	}))
	require.NoError(t, validateDirectUSDTOptionValues(map[string]string{
		"USDTReceivingWallets": `[{
			"network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
		}]`,
	}))
}
