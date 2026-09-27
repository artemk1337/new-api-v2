package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUSDTReceivingWalletsFailsClosed(t *testing.T) {
	for _, value := range []string{"{", `[{"network":"TRON","address":"bad"}]`, `[{"key":"x","network":"BTC","address":"x"}]`, `[{"network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8","destination":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}]`} {
		_, err := ParseUSDTReceivingWallets(value)
		require.Error(t, err)
	}
}

func TestParseUSDTReceivingWalletsNormalizesTON(t *testing.T) {
	raw := "0:B113A994B5024A16719F69139328EB759596C38A25F59028B146FECDC3621DFE"
	wallets, err := ParseUSDTReceivingWallets(`[{"key":"ton-1","network":"ton","address":"` + raw + `"}]`)
	require.NoError(t, err)
	require.Len(t, wallets, 1)
	require.Equal(t, "TON", wallets[0].Network)
	require.Equal(t, raw, wallets[0].Address)
}

func TestParseUSDTReceivingWalletsKeepsExplicitEmptyArray(t *testing.T) {
	wallets, err := ParseUSDTReceivingWallets("[]")
	require.NoError(t, err)
	require.Empty(t, wallets)
}
