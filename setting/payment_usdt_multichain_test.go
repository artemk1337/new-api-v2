package setting

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalTONAddressRawAndFriendly(t *testing.T) {
	raw := "0:" + "B113A994B5024A16719F69139328EB759596C38A25F59028B146FECDC3621DFE"
	require.NoError(t, ValidateTONAddress(raw))
	// Build a valid non-testnet friendly address from the same hash.
	payload := append([]byte{0x11, 0}, mustDecodeHex(t, raw[2:])...)
	crc := tonCRC(payload)
	friendly := base64.RawURLEncoding.EncodeToString(append(payload, crc...))
	canonical, err := CanonicalTONAddress(friendly)
	require.NoError(t, err)
	require.Equal(t, raw, canonical)
	require.Error(t, ValidateTONAddress("0:"+"00"))
}

func tonCRC(payload []byte) []byte {
	var crc uint16
	for _, b := range payload {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return []byte{byte(crc >> 8), byte(crc)}
}
func mustDecodeHex(t *testing.T, value string) []byte {
	b, err := hex.DecodeString(value)
	require.NoError(t, err)
	return b
}

func TestValidateSolanaAddress(t *testing.T) {
	require.NoError(t, ValidateSolanaAddress("11111111111111111111111111111111"))
	require.Error(t, ValidateSolanaAddress("not-a-solana-address"))
}

func TestValidateUSDTProviderEndpointAllowlist(t *testing.T) {
	require.NoError(t, ValidateUSDTProviderEndpoint("TON", "https://toncenter.com/api/v3"))
	require.Error(t, ValidateUSDTProviderEndpoint("TON", "http://toncenter.com/api/v3"))
	require.Error(t, ValidateUSDTProviderEndpoint("TON", "https://evil.example/api/v3"))
	require.NoError(t, ValidateUSDTProviderEndpoint("SOLANA", "https://api.mainnet-beta.solana.com"))
	require.Error(t, ValidateUSDTProviderEndpoint("SOLANA", "https://127.0.0.1"))
}

func TestValidateSolanaTokenAccountFailsClosedOnWrongOwner(t *testing.T) {
	oldURL, oldClient := USDTSolanaRPCURL, USDTSolanaHTTPClient
	defer func() { USDTSolanaRPCURL = oldURL; USDTSolanaHTTPClient = oldClient }()
	owner := "11111111111111111111111111111111"
	destination := "11111111111111111111111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"value":[{"pubkey":"11111111111111111111111111111111","account":{"owner":"TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA","data":{"parsed":{"info":{"owner":"Bad111111111111111111111111111111111111111","mint":"Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB","tokenAmount":{"decimals":6}}}}}}]}}`))
	}))
	defer server.Close()
	USDTSolanaRPCURL = server.URL
	USDTSolanaHTTPClient = server.Client()
	require.Error(t, ValidateSolanaTokenAccount(owner, destination))
}
