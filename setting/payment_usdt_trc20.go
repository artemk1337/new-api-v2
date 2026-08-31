package setting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Direct USDT TRC20 payments are deliberately disabled until an operator
// configures and explicitly enables the receiving address and TronGrid key.
// The API key is only a read-only TronGrid credential; no wallet private key
// is ever accepted or stored by this integration.
var (
	USDTTRC20Enabled          = false
	USDTTRC20ReceivingAddress = ""
	// USDTTONReceivingAddress and USDTSolanaReceivingAddress are staged
	// receiving addresses for future network integrations. They are persisted
	// and exposed in admin settings, but no payment flow currently reads them.
	USDTTONReceivingAddress      = ""
	USDTSolanaReceivingAddress   = ""
	USDTTRC20APIKey              = ""
	USDTTRC20MinBaseUSD          = "10"
	USDTTRC20MaxCreationsPerHour = 0
	USDTTRC20PaymentURLBase      = ""
	USDTTRC20MinConfirmations    = 19
	// USDTTRC20AmountTailLimitUnits is the exclusive upper bound for the
	// random suffix in micro-USDT units. A generated suffix is always in
	// [1, limit), so it is non-zero and strictly below the configured limit.
	USDTTRC20AmountTailLimitUnits = DefaultUSDTTRC20AmountTailLimitUnits
	// USDTTRC20AmountPrecision is retained only as a migration input for old
	// installations. It is never consulted by payment runtime after migration.
	USDTTRC20AmountPrecision = DefaultUSDTTRC20AmountPrecision
	// These legacy fields are kept only so old installations can load their
	// options. They are not read by the direct payment runtime anymore.
	USDTTRC20AmountSuffixMinUnits = DefaultUSDTTRC20AmountSuffixMinUnits
	USDTTRC20AmountSuffixMaxUnits = DefaultUSDTTRC20AmountSuffixMaxUnits
)

const (
	// Official Tether USD contract on TRON mainnet. This is immutable by design.
	USDTTRC20Contract = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	// TronGrid's public API is deliberately fixed to avoid accepting an
	// operator-controlled endpoint that could forge payment events.
	USDTTRC20TronGridAPIBaseURL          = "https://api.trongrid.io"
	USDTTRC20MaxCreations                = 1000
	USDTTRC20AmountSuffixMax             = 9999
	USDTTRC20AmountTailLimitMin          = 2
	USDTTRC20AmountTailLimitMax          = 10_000
	DefaultUSDTTRC20AmountPrecision      = 6
	DefaultUSDTTRC20AmountTailLimitUnits = 10_000
	DefaultUSDTTRC20AmountSuffixMinUnits = 1
	DefaultUSDTTRC20AmountSuffixMaxUnits = 9999
)

// ValidateUSDTTRC20AmountTailLimit validates the exclusive upper bound for
// random exact-amount suffixes.
func ValidateUSDTTRC20AmountTailLimit(limit int) error {
	if limit < USDTTRC20AmountTailLimitMin || limit > USDTTRC20AmountTailLimitMax {
		return fmt.Errorf("USDT TRC20 amount tail limit must be between %d and %d micro-USDT", USDTTRC20AmountTailLimitMin, USDTTRC20AmountTailLimitMax)
	}
	return nil
}

// ValidateUSDTTRC20AmountPrecision validates the legacy migration setting.
func ValidateUSDTTRC20AmountPrecision(precision int) error {
	if precision < 3 || precision > 6 {
		return fmt.Errorf("USDT TRC20 amount precision must be between 3 and 6 decimal places")
	}
	return nil
}

// USDTTRC20AmountTailLimitForPrecision maps the old discrete precision to
// the equivalent exclusive tail limit.
func USDTTRC20AmountTailLimitForPrecision(precision int) (int, error) {
	if err := ValidateUSDTTRC20AmountPrecision(precision); err != nil {
		return 0, err
	}
	limits := [...]int{3: 10, 4: 100, 5: 1_000, 6: 10_000}
	return limits[precision], nil
}

// USDTTRC20AmountSuffixRange returns the inclusive runtime suffix range.
func USDTTRC20AmountSuffixRange() (minUnits, maxUnits, stepUnits int, err error) {
	return USDTTRC20AmountSuffixRangeForLimit(USDTTRC20AmountTailLimitUnits)
}

func USDTTRC20AmountSuffixRangeForLimit(limit int) (minUnits, maxUnits, stepUnits int, err error) {
	if err = ValidateUSDTTRC20AmountTailLimit(limit); err != nil {
		return 0, 0, 0, err
	}
	return 1, limit - 1, 1, nil
}

// ValidateUSDTTRC20AmountSuffixRange validates the inclusive random suffix
// range used to distinguish exact payment amounts. Values are micro-USDT
// units and must stay below 0.01 USDT.
func ValidateUSDTTRC20AmountSuffixRange(minUnits, maxUnits int) error {
	if minUnits < 1 || maxUnits < 1 || minUnits > maxUnits || maxUnits > USDTTRC20AmountSuffixMax {
		return fmt.Errorf("USDT TRC20 amount suffix range must satisfy 1 <= min <= max <= %d", USDTTRC20AmountSuffixMax)
	}
	return nil
}

// ValidateTRONAddress validates a mainnet TRON Base58Check account address.
// TRON addresses have version 0x41 and a 20-byte account payload.
func ValidateTRONAddress(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("TRON receiving address is required")
	}
	// The token contract is not an account and cannot receive TRC20
	// settlements.  It is a valid Base58Check string, so this guard must run
	// before the generic address validation.
	if strings.EqualFold(address, USDTTRC20Contract) {
		return errors.New("TRON receiving address must not be the USDT contract")
	}
	decoded, err := decodeBase58(address)
	if err != nil || len(decoded) != 25 || decoded[0] != 0x41 {
		return errors.New("invalid TRON receiving address")
	}
	digest := sha256.Sum256(decoded[:21])
	digest = sha256.Sum256(digest[:])
	if !strings.EqualFold(hex.EncodeToString(digest[:4]), hex.EncodeToString(decoded[21:])) {
		return errors.New("invalid TRON receiving address checksum")
	}
	return nil
}

func decodeBase58(value string) ([]byte, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	if value == "" {
		return nil, errors.New("empty base58 value")
	}
	// Decode through a big integer.  The previous byte-wise implementation
	// propagated carries in the wrong direction for many perfectly valid
	// TRON addresses, causing checksum validation to reject real wallets.
	result := new(big.Int)
	base := big.NewInt(58)
	for _, char := range value {
		index := strings.IndexRune(alphabet, char)
		if index < 0 {
			return nil, fmt.Errorf("invalid base58 character")
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(index)))
	}
	decoded := result.Bytes()
	leadingZeros := 0
	for leadingZeros < len(value) && value[leadingZeros] == '1' {
		leadingZeros++
	}
	return append(make([]byte, leadingZeros), decoded...), nil
}

// ValidateDirectUSDTConfigValues is used before an option update is committed
// so an enabled integration can never be persisted without a usable address
// and a read-only TronGrid credential.
func ValidateDirectUSDTConfigValues(enabled bool, address, apiKey string) error {
	if !enabled {
		return nil
	}
	if err := ValidateTRONAddress(address); err != nil {
		return err
	}
	if strings.TrimSpace(apiKey) == "" {
		return errors.New("TronGrid API key is required when USDT TRC20 is enabled")
	}
	return nil
}
