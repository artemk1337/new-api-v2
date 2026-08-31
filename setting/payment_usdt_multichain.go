package setting

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// USDT TON/Solana integration settings are read-only watcher credentials and
// receiving addresses. Private keys are intentionally not supported.
var (
	USDTTONAPIKey                   = ""
	USDTTONAPIBaseURL               = "https://toncenter.com/api/v3"
	USDTSolanaRPCURL                = "https://api.mainnet-beta.solana.com"
	USDTSolanaAPIKey                = ""
	USDTSolanaReceivingTokenAccount = ""
	USDTSolanaHTTPClient            = &http.Client{Timeout: 15 * time.Second}
)

const (
	USDTTONJettonMaster    = "0:B113A994B5024A16719F69139328EB759596C38A25F59028B146FECDC3621DFE"
	USDTTONDecimals        = 6
	USDTSolanaMint         = "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"
	USDTSolanaTokenProgram = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
)

// ValidateTONAddress accepts a mainnet TON friendly EQ/UQ address or raw
// workchain-0 address and returns its canonical raw form. Testnet and other
// workchains are rejected to prevent funds being sent to an unusable wallet.
func ValidateTONAddress(address string) error {
	_, err := CanonicalTONAddress(address)
	return err
}

func CanonicalTONAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", errors.New("TON receiving address is required")
	}
	if strings.HasPrefix(address, "0:") || strings.HasPrefix(address, "-1:") {
		parts := strings.Split(address, ":")
		if len(parts) != 2 || parts[0] != "0" || len(parts[1]) != 64 {
			return "", errors.New("TON address must be workchain 0")
		}
		if _, err := hex.DecodeString(parts[1]); err != nil {
			return "", errors.New("invalid TON address")
		}
		return "0:" + strings.ToUpper(parts[1]), nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(address, "="))
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(address)
	}
	if err != nil || len(decoded) != 36 {
		return "", errors.New("invalid TON friendly address")
	}
	flags, wc := decoded[0], int8(decoded[1])
	if flags&0x80 != 0 || (flags&0x3f != 0x11 && flags&0x3f != 0x51) || wc != 0 {
		return "", errors.New("TON address must be mainnet workchain 0")
	}
	if !tonCRCValid(decoded[:34], decoded[34:]) {
		return "", errors.New("invalid TON address checksum")
	}
	return "0:" + strings.ToUpper(hex.EncodeToString(decoded[2:34])), nil
}

func tonCRCValid(payload, checksum []byte) bool {
	if len(checksum) != 2 {
		return false
	}
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
	return binary.BigEndian.Uint16(checksum) == crc
}

// ValidateSolanaAddress validates a legacy mainnet base58 account address.
func ValidateSolanaAddress(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("Solana receiving address is required")
	}
	decoded, err := decodeSolanaBase58(address)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid Solana address")
	}
	return nil
}

func decodeSolanaBase58(value string) ([]byte, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int)
	for _, r := range value {
		i := strings.IndexRune(alphabet, r)
		if i < 0 {
			return nil, errors.New("invalid base58 character")
		}
		n.Mul(n, big.NewInt(58))
		n.Add(n, big.NewInt(int64(i)))
	}
	b := n.Bytes()
	zeros := 0
	for zeros < len(value) && value[zeros] == '1' {
		zeros++
	}
	return append(make([]byte, zeros), b...), nil
}

func ValidateDirectUSDTMultiChainConfig(network, address, apiKey string) error {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TRON":
		return ValidateDirectUSDTConfigValues(true, address, apiKey)
	case "TON":
		if err := ValidateTONAddress(address); err != nil {
			return err
		}
		if strings.TrimSpace(apiKey) == "" {
			return errors.New("Toncenter API key is required when USDT TON is enabled")
		}
	case "SOLANA":
		if err := ValidateSolanaAddress(address); err != nil {
			return err
		}
		if strings.TrimSpace(apiKey) == "" {
			return errors.New("Solana RPC API key is required when USDT Solana is enabled")
		}
		if strings.TrimSpace(USDTSolanaReceivingTokenAccount) == "" {
			return errors.New("Solana USDT token account is required when enabled")
		}
		if err := ValidateSolanaAddress(USDTSolanaReceivingTokenAccount); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported USDT network %q", network)
	}
	return nil
}

func ValidateUSDTProviderEndpoint(network, endpoint string) error {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("provider endpoint must use HTTPS without credentials or query")
	}
	host := strings.ToLower(u.Hostname())
	path := strings.TrimRight(u.Path, "/")
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TON":
		if host != "toncenter.com" || path != "/api/v3" {
			return errors.New("TON endpoint must be https://toncenter.com/api/v3")
		}
	case "SOLANA":
		if host != "api.mainnet-beta.solana.com" || (path != "" && path != "/") {
			return errors.New("Solana endpoint must be https://api.mainnet-beta.solana.com")
		}
	default:
		return errors.New("unsupported network")
	}
	return nil
}

// ValidateSolanaTokenAccount performs a finalized RPC proof that the
// configured destination is a legacy SPL account for the canonical USDT mint
// and is owned by the configured wallet owner. RPC errors fail closed.
func ValidateSolanaTokenAccount(owner, destination string) error {
	if err := ValidateSolanaAddress(owner); err != nil {
		return err
	}
	if err := ValidateSolanaAddress(destination); err != nil {
		return err
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "getTokenAccountsByOwner", "params": []any{owner, map[string]any{"mint": USDTSolanaMint}, map[string]any{"encoding": "jsonParsed", "commitment": "finalized"}}}
	body, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, USDTSolanaRPCURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(USDTSolanaAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := USDTSolanaHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Solana RPC returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Result struct {
			Value []struct {
				Pubkey  string `json:"pubkey"`
				Account struct {
					Owner string `json:"owner"`
					Data  struct {
						Parsed struct {
							Info struct {
								Owner       string `json:"owner"`
								Mint        string `json:"mint"`
								TokenAmount struct {
									Decimals int `json:"decimals"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := common.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return errors.New("Solana RPC returned an error")
	}
	for _, v := range envelope.Result.Value {
		if v.Pubkey == destination && v.Account.Owner == USDTSolanaTokenProgram && v.Account.Data.Parsed.Info.Owner == owner && v.Account.Data.Parsed.Info.Mint == USDTSolanaMint && v.Account.Data.Parsed.Info.TokenAmount.Decimals == USDTTONDecimals {
			return nil
		}
	}
	return errors.New("Solana destination token account does not match owner, mint or token program")
}

// SolanaAddressDigest is useful when persisting an immutable token-account
// snapshot without exposing account internals in logs.
func SolanaAddressDigest(address string) string {
	d := sha256.Sum256([]byte(strings.TrimSpace(address)))
	return hex.EncodeToString(d[:])
}
