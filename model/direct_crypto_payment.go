package model

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	// DirectCryptoProvider is the sole provider ID for all newly-created direct
	// USDT invoices. Network-specific values below remain readable historical
	// provider IDs so pending rows created by an older binary reconcile safely.
	DirectCryptoProvider    = operation_setting.DirectCryptoPaymentMethod
	DirectUSDTTRC20Provider = operation_setting.DirectUSDTTRC20PaymentMethod
)

const (
	// DirectUSDTMinBaseUnits is the minimum base amount accepted by the
	// checkout.  The random suffix is added after this value and is never
	// included in quota accounting.
	DirectUSDTMinBaseUnits uint64 = 10_000_000 // $10.000000
	// Direct orders must remain inside the reconciliation overlap.  Keeping
	// this invariant in the model protects callers that bypass the HTTP
	// controller and prevents an order from outliving the watcher horizon.
	DirectUSDTMaxPendingSeconds int64 = int64(operation_setting.MaxDirectUSDTTRC20PendingTTL / time.Second)

	DirectCryptoPending = "pending"
	DirectCryptoPaid    = "paid"
	DirectCryptoExpired = "expired"
	DirectCryptoFailed  = "failed"
)

var (
	ErrDirectPaymentDisabled         = errors.New("direct USDT TRC20 payments are disabled")
	ErrDirectPaymentInvalid          = errors.New("invalid direct USDT TRC20 payment event")
	ErrDirectPaymentExpired          = errors.New("direct USDT TRC20 payment expired")
	ErrDirectPaymentAmountMismatch   = errors.New("direct USDT TRC20 payment amount mismatch")
	ErrDirectPaymentAlreadySettled   = errors.New("direct USDT TRC20 payment already settled")
	ErrDirectPaymentEventAlreadyUsed = errors.New("direct USDT TRC20 event already belongs to another order")
	ErrDirectPaymentLimitExceeded    = errors.New("direct USDT TRC20 payment creation limit exceeded")
	ErrDirectPaymentAmountExhausted  = errors.New("direct USDT TRC20 exact amount space exhausted for this base amount")
)

// DirectCryptoPayment is an immutable amount reservation plus the observed
// chain event. ExpectedUnits is in USDT's six-decimal smallest unit and is
// unique for the lifetime of the database: an expired order is intentionally
// never reused because a late transfer must not settle a newer order.
type DirectCryptoPayment struct {
	Id       uint   `gorm:"primaryKey" json:"id"`
	TradeNo  string `gorm:"uniqueIndex;type:varchar(255);not null" json:"trade_no"`
	UserId   int    `gorm:"index;not null" json:"user_id"`
	Network  string `gorm:"type:varchar(16);not null" json:"network"`
	Token    string `gorm:"type:varchar(16);not null" json:"token"`
	Contract string `gorm:"type:varchar(128);not null" json:"contract"`
	Address  string `gorm:"type:varchar(128);not null" json:"address"`
	// ReceivingOwner is the wallet owner. Destination is the concrete token
	// account (Solana SPL) or owner destination (TON); both are immutable
	// snapshots captured at invoice creation. Legacy TRON rows leave them empty.
	ReceivingOwner string `gorm:"type:varchar(128)" json:"receiving_owner,omitempty"`
	Destination    string `gorm:"type:varchar(128)" json:"destination,omitempty"`
	ExpectedUnits  uint64 `gorm:"not null;uniqueIndex:idx_direct_crypto_payment_expected_units" json:"exact_amount_units"`
	BaseUnits      uint64 `gorm:"not null" json:"base_amount_units"`
	SuffixUnits    uint32 `gorm:"not null" json:"suffix_units"`
	Status         string `gorm:"type:varchar(16);index;not null" json:"status"`
	TxHash         string `gorm:"type:varchar(128);index:idx_direct_crypto_payment_tx_hash" json:"tx_hash,omitempty"`
	EventIndex     string `gorm:"type:varchar(64)" json:"event_index,omitempty"`
	EventID        string `gorm:"type:varchar(255);index" json:"event_id,omitempty"`
	ObservedUnits  uint64 `json:"observed_units,omitempty"`
	Confirmations  uint64 `json:"confirmations,omitempty"`
	ExpiresAt      int64  `gorm:"index;not null" json:"expires_at"`
	CreatedAt      int64  `gorm:"index;not null" json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// DirectUSDTTransferEvent is produced only by the TronGrid watcher. The
// watcher requests only confirmed TRC20 transfers, but settlement repeats all
// security checks against the immutable order snapshot before crediting.
type DirectUSDTTransferEvent struct {
	TradeNo        string
	TxHash         string
	EventIndex     string
	EventID        string
	Contract       string
	To             string
	AmountUnits    uint64
	Confirmations  uint64
	Confirmed      bool
	BlockTimestamp int64
	Network        string
	Destination    string
	Decimals       uint8
}

func (p *DirectCryptoPayment) Insert(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return tx.Create(p).Error
}

func GetDirectCryptoPayment(tradeNo string) (*DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var p DirectCryptoPayment
	if err := DB.Where("trade_no = ?", strings.TrimSpace(tradeNo)).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func GetPendingDirectUSDTPayments(expectedUnits uint64, address string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("expected_units = ? AND address = ? AND (status = ? OR (status = ? AND expires_at >= ?))", expectedUnits, strings.TrimSpace(address), DirectCryptoPending, DirectCryptoExpired, cutoff).
		Find(&payments).Error
	return payments, err
}

func GetPendingDirectUSDTNetworkPayments(network string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("network = ? AND (status = ? OR (status = ? AND expires_at >= ?))", strings.ToUpper(strings.TrimSpace(network)), DirectCryptoPending, DirectCryptoExpired, cutoff).Find(&payments).Error
	return payments, err
}

// GetActivePendingDirectUSDTPaymentAddresses returns every receiving address
// captured by a pending direct order. The address is part of the immutable
// order snapshot; using only the current setting would orphan orders created
// before an operator rotated the wallet.
func GetActivePendingDirectUSDTPaymentAddresses(now int64) ([]string, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var addresses []string
	cutoff := now - int64(operation_setting.MaxDirectUSDTTRC20PendingTTL/time.Second)
	// A pending row may be stale because the periodic sweep has not run yet, so
	// keep all pending snapshots. Expired snapshots are retained only while a
	// pre-expiry chain event could still be inside the watcher's overlap window.
	err := DB.Model(&DirectCryptoPayment{}).
		Where("address <> '' AND (status = ? OR (status = ? AND expires_at >= ?))", DirectCryptoPending, DirectCryptoExpired, cutoff).
		Distinct("address").Pluck("address", &addresses).Error
	return addresses, err
}

func (p *DirectCryptoPayment) Expired(now int64) bool {
	return p != nil && p.Status == DirectCryptoPending && p.ExpiresAt > 0 && now >= p.ExpiresAt
}

// pendingTopUpExpired uses the immutable direct-payment deadline when the
// top-up belongs to this integration.  Other gateways intentionally keep the
// existing dynamic TTL policy.  A missing direct snapshot is an error rather
// than permission to expire or settle by a guessed deadline.
func pendingTopUpExpired(tx *gorm.DB, topUp *TopUp, now int64) (bool, error) {
	if topUp == nil || !isDirectUSDTNetworkProvider(topUp.PaymentProvider) {
		if topUp != nil && topUp.PaymentPendingTTLSeconds > 0 && topUp.CreateTime > 0 {
			return now-topUp.CreateTime >= topUp.PaymentPendingTTLSeconds, nil
		}
		return topUp != nil && topUp.CreateTime > 0 &&
			now-topUp.CreateTime >= int64(operation_setting.PendingTopUpTTL(topUp.PaymentMethod)/time.Second), nil
	}
	var payment DirectCryptoPayment
	query := tx.Where("trade_no = ?", strings.TrimSpace(topUp.TradeNo))
	if tx.Dialector.Name() != "sqlite" {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	if err := query.First(&payment).Error; err != nil {
		return false, err
	}
	return (payment.Status == DirectCryptoExpired ||
		(payment.Status == DirectCryptoPending && payment.ExpiresAt > 0 && now >= payment.ExpiresAt)), nil
}

// ValidateDirectUSDTConfig checks the runtime configuration before an order is
// created. PayMethods is the activation source of truth; the old Enabled flag
// is consulted only by the one-time migration path.
func ValidateDirectUSDTConfig() error {
	return setting.ValidateDirectUSDTConfigValues(true, setting.USDTTRC20ReceivingAddress, setting.USDTTRC20APIKey)
}

func paymentProviderForNetwork(_ *DirectCryptoPayment) string { return DirectCryptoProvider }

func directUSDTReceivingAddress(network string) string {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TON":
		if raw, err := setting.CanonicalTONAddress(setting.USDTTONReceivingAddress); err == nil {
			return raw
		}
		return strings.TrimSpace(setting.USDTTONReceivingAddress)
	case "SOLANA":
		return strings.TrimSpace(setting.USDTSolanaReceivingAddress)
	default:
		return strings.TrimSpace(setting.USDTTRC20ReceivingAddress)
	}
}

func validDirectUSDTNetwork(network, contract string) bool {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TRON":
		return contract == setting.USDTTRC20Contract
	case "TON":
		return strings.EqualFold(contract, setting.USDTTONJettonMaster)
	case "SOLANA":
		return contract == setting.USDTSolanaMint
	default:
		return false
	}
}

func ValidateDirectUSDTNetworkConfig(network, address string) error {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TRON":
		return setting.ValidateDirectUSDTConfigValues(true, address, setting.USDTTRC20APIKey)
	case "TON":
		if err := setting.ValidateUSDTProviderEndpoint("TON", setting.USDTTONAPIBaseURL); err != nil {
			return err
		}
		return setting.ValidateDirectUSDTMultiChainConfig("TON", address, setting.USDTTONAPIKey)
	case "SOLANA":
		if err := setting.ValidateUSDTProviderEndpoint("SOLANA", setting.USDTSolanaRPCURL); err != nil {
			return err
		}
		if err := setting.ValidateDirectUSDTMultiChainConfig("SOLANA", address, setting.USDTSolanaAPIKey); err != nil {
			return err
		}
		return setting.ValidateSolanaTokenAccount(address, setting.USDTSolanaReceivingTokenAccount)
	default:
		return errors.New("unsupported USDT network")
	}
}

func DirectUSDTNetworkReady(network string) bool {
	address := directUSDTReceivingAddress(network)
	if err := ValidateDirectUSDTNetworkConfig(network, address); err != nil {
		return false
	}
	if strings.EqualFold(network, "SOLANA") {
		return setting.ValidateSolanaTokenAccount(address, setting.USDTSolanaReceivingTokenAccount) == nil
	}
	return true
}

// DirectUSDTReadyNetworks is the only source for checkout network choices.
// It intentionally probes each network's complete read-only validation path
// and returns a stable order for API consumers.
func DirectUSDTReadyNetworks() []string {
	ready := make([]string, 0, 3)
	for _, network := range []string{"TRON", "TON", "SOLANA"} {
		if DirectUSDTNetworkReady(network) {
			ready = append(ready, network)
		}
	}
	return ready
}

func DirectUSDTNetworkIsReady(network string) bool {
	for _, ready := range DirectUSDTReadyNetworks() {
		if ready == strings.ToUpper(strings.TrimSpace(network)) {
			return true
		}
	}
	return false
}

func IsDirectUSDTNetworkMethodConfigured(provider string) bool {
	methods, err := GetPayMethodsFromDB(DB)
	if err == nil {
		for _, m := range methods {
			if m != nil && strings.EqualFold(strings.TrimSpace(m["type"]), provider) {
				return true
			}
		}
		return false
	}
	return errors.Is(err, gorm.ErrRecordNotFound) &&
		(provider == DirectCryptoProvider || provider == DirectUSDTTRC20Provider) && setting.USDTTRC20Enabled
}

// CaptureDirectCryptoPolicy snapshots the one parent method for a newly
// created invoice. It reads the persisted catalog instead of network-specific
// legacy entries, so min_topup and TTL cannot vary by selected chain.
func CaptureDirectCryptoPolicy(topUp *TopUp) error {
	methods, err := GetPayMethodsFromDB(DB)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		methods = operation_setting.CanonicalizePayMethods(operation_setting.PayMethodsSnapshot())
		if setting.USDTTRC20Enabled && !HasDirectUSDTMethod(methods) {
			methods = append(methods, map[string]string{"type": DirectCryptoProvider, "name": "Crypto"})
		}
	} else if err != nil {
		return err
	}
	for _, method := range methods {
		if method == nil || !strings.EqualFold(strings.TrimSpace(method["type"]), DirectCryptoProvider) {
			continue
		}
		minimum := 10.0
		if value, parseErr := strconv.ParseFloat(strings.TrimSpace(method["min_topup"]), 64); parseErr == nil && value >= minimum {
			minimum = value
		}
		ttl := operation_setting.DefaultDirectUSDTTRC20PendingTTL
		if minutes, parseErr := strconv.Atoi(strings.TrimSpace(method["pending_ttl_minutes"])); parseErr == nil && minutes > 0 {
			ttl = time.Duration(min(minutes, int(operation_setting.MaxDirectUSDTTRC20PendingTTL/time.Minute))) * time.Minute
		}
		topUp.PaymentMinimumAmount = minimum
		topUp.PaymentPendingTTLSeconds = int64(ttl / time.Second)
		return nil
	}
	return ErrDirectPaymentDisabled
}

func DirectCryptoNow() int64 { return time.Now().Unix() }

// CreateDirectUSDTOrder atomically reserves a unique exact amount and creates
// the matching TopUp row. A user row lock serializes the hourly creation limit
// across application instances; the expected-amount unique index closes the
// remaining allocation race between different users.
func CreateDirectUSDTOrder(topUp *TopUp, payment *DirectCryptoPayment) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if topUp == nil || payment == nil {
		return errors.New("invalid direct USDT order")
	}
	provider := DirectCryptoProvider
	if !IsDirectUSDTNetworkMethodConfigured(provider) || !DirectUSDTNetworkIsReady(payment.Network) {
		return ErrDirectPaymentDisabled
	}
	if err := ValidateDirectUSDTNetworkConfig(payment.Network, payment.Address); err != nil {
		return err
	}
	if topUp.UserId == 0 || payment.UserId != topUp.UserId || payment.BaseUnits < DirectUSDTMinBaseUnits {
		return errors.New("invalid direct USDT TRC20 order")
	}
	if strings.TrimSpace(topUp.TradeNo) == "" || strings.TrimSpace(payment.TradeNo) != strings.TrimSpace(topUp.TradeNo) ||
		!strings.EqualFold(strings.TrimSpace(topUp.PaymentMethod), provider) ||
		!strings.EqualFold(strings.TrimSpace(topUp.PaymentProvider), provider) ||
		topUp.Status != common.TopUpStatusPending || topUp.CreateTime <= 0 || topUp.CreateTime != payment.CreatedAt {
		return errors.New("invalid direct USDT TRC20 order")
	}
	if payment.Token != "USDT" || !validDirectUSDTNetwork(payment.Network, payment.Contract) {
		return errors.New("direct USDT order has invalid immutable asset")
	}
	payment.Address = strings.TrimSpace(payment.Address)
	configuredAddress := directUSDTReceivingAddress(payment.Network)
	if strings.EqualFold(payment.Network, "TON") {
		if canonical, err := setting.CanonicalTONAddress(payment.Address); err == nil {
			payment.Address = canonical
		}
		if canonical, err := setting.CanonicalTONAddress(payment.ReceivingOwner); err == nil {
			payment.ReceivingOwner = canonical
		}
		if canonical, err := setting.CanonicalTONAddress(payment.Destination); err == nil {
			payment.Destination = canonical
		}
	}
	if payment.Address != configuredAddress {
		return errors.New("direct USDT TRC20 order address does not match configured receiving address")
	}
	if err := ValidateDirectUSDTNetworkConfig(payment.Network, payment.Address); err != nil {
		return err
	}
	if payment.ExpiresAt <= payment.CreatedAt || payment.CreatedAt <= 0 ||
		payment.ExpiresAt-payment.CreatedAt > DirectUSDTMaxPendingSeconds {
		return errors.New("direct USDT TRC20 order has invalid expiry")
	}
	if err := CaptureDirectCryptoPolicy(topUp); err != nil {
		return err
	}
	payment.ExpiresAt = payment.CreatedAt + topUp.PaymentPendingTTLSeconds
	if payment.ExpiresAt <= payment.CreatedAt || payment.CreatedAt <= 0 ||
		payment.ExpiresAt-payment.CreatedAt > DirectUSDTMaxPendingSeconds {
		return errors.New("direct USDT TRC20 order has invalid expiry")
	}
	if baseUSD := float64(payment.BaseUnits) / 1_000_000; baseUSD < topUp.PaymentMinimumAmount {
		return errors.New("direct USDT amount is below the configured method minimum")
	}
	suffixMinUnits, suffixMaxUnits, suffixStepUnits, err := setting.USDTTRC20AmountSuffixRange()
	if err != nil {
		return err
	}
	suffixRangeSize := int64((suffixMaxUnits-suffixMinUnits)/suffixStepUnits + 1)

	const maxAttempts = 32
	for attempt := 0; attempt < maxAttempts; attempt++ {
		suffix, err := randomDirectUSDTTRC20Suffix()
		if err != nil {
			return err
		}
		payment.Id = 0
		topUp.Id = 0
		payment.SuffixUnits = suffix
		payment.ExpectedUnits = payment.BaseUnits + uint64(suffix)
		if payment.ExpectedUnits <= payment.BaseUnits {
			return errors.New("direct USDT TRC20 amount overflow")
		}
		// Record the exact provider amount in the immutable TopUp snapshot. The
		// suffix is an identity discriminator, not a commission, so quota stays
		// based on BaseUnits while settlement still compares the full amount.
		expectedAmount, parseErr := strconv.ParseFloat(DirectUSDTAmountString(payment.ExpectedUnits), 64)
		if parseErr != nil {
			return parseErr
		}
		topUp.PaymentChargedAmount = expectedAmount
		topUp.Money = expectedAmount

		err = DB.Transaction(func(tx *gorm.DB) error {
			if err := lockDirectUSDTUser(tx, topUp.UserId); err != nil {
				return err
			}
			if limit := setting.USDTTRC20MaxCreationsPerHour; limit > 0 {
				var count int64
				cutoff := payment.CreatedAt - int64(time.Hour/time.Second)
				if err := tx.Model(&DirectCryptoPayment{}).
					Where("user_id = ? AND created_at >= ?", topUp.UserId, cutoff).
					Count(&count).Error; err != nil {
					return err
				}
				if count >= int64(limit) {
					return ErrDirectPaymentLimitExceeded
				}
			}
			var usedSuffixes []uint32
			if err := tx.Model(&DirectCryptoPayment{}).
				Where("base_units = ? AND suffix_units >= ? AND suffix_units <= ?", payment.BaseUnits, suffixMinUnits, suffixMaxUnits).
				Pluck("suffix_units", &usedSuffixes).Error; err != nil {
				return err
			}
			used := int64(0)
			for _, usedSuffix := range usedSuffixes {
				if int(usedSuffix) >= suffixMinUnits && int(usedSuffix) <= suffixMaxUnits && (int(usedSuffix)-suffixMinUnits)%suffixStepUnits == 0 {
					used++
				}
			}
			// Every configured suffix is permanently quarantined after use so a
			// late transfer can never be mistaken for a newer order.
			if used >= suffixRangeSize {
				return ErrDirectPaymentAmountExhausted
			}
			if err := tx.Create(topUp).Error; err != nil {
				return err
			}
			return tx.Create(payment).Error
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrDirectPaymentLimitExceeded) {
			return err
		}
		if errors.Is(err, ErrDirectPaymentAmountExhausted) {
			return err
		}
		if isSQLiteBusyError(err) || isUniqueConstraintError(err) {
			continue
		}
		return err
	}
	return ErrDirectPaymentAmountExhausted
}

func lockDirectUSDTUser(tx *gorm.DB, userID int) error {
	query := tx.Select("id").Where("id = ?", userID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	var user User
	if err := query.First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTopUpUserNotFound
		}
		return err
	}
	return nil
}

var directUSDTTRC20RandRead = rand.Read

func randomDirectUSDTTRC20Suffix() (uint32, error) {
	// crypto/rand is used rather than math/rand so concurrent workers cannot
	// predict or intentionally collide with a user's amount reservation. Use
	// rejection sampling instead of modulo alone so every configured suffix is
	// equally likely even when the range does not divide 2^16.
	minUnits, maxUnits, stepUnits, err := setting.USDTTRC20AmountSuffixRange()
	if err != nil {
		return 0, err
	}
	rangeSize := uint32((maxUnits-minUnits)/stepUnits + 1)
	const sampleSpace = uint32(1 << 16)
	limit := sampleSpace - sampleSpace%rangeSize
	for attempts := 0; attempts < 128; attempts++ {
		var bytes [2]byte
		if _, err := directUSDTTRC20RandRead(bytes[:]); err != nil {
			return 0, err
		}
		sample := uint32(uint16(bytes[0])<<8 | uint16(bytes[1]))
		if sample >= limit {
			continue
		}
		return uint32(minUnits) + (sample%rangeSize)*uint32(stepUnits), nil
	}
	return 0, errors.New("failed to sample a direct USDT amount suffix")
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") || strings.Contains(message, "constraint failed")
}

func isDirectPaymentEventValid(event DirectUSDTTransferEvent) bool {
	minConfirmations := setting.USDTTRC20MinConfirmations
	if !strings.EqualFold(event.Network, "TRON") {
		minConfirmations = 0
	}
	return strings.TrimSpace(event.TradeNo) != "" &&
		strings.TrimSpace(event.TxHash) != "" &&
		strings.TrimSpace(event.EventID) != "" &&
		event.EventID == DirectUSDTEventID(event.TxHash, event.EventIndex) &&
		event.AmountUnits > 0 && event.Confirmed &&
		(minConfirmations <= 0 || event.Confirmations >= uint64(minConfirmations))
}

func (event DirectUSDTTransferEvent) normalized() DirectUSDTTransferEvent {
	event.TradeNo = strings.TrimSpace(event.TradeNo)
	event.TxHash = strings.TrimSpace(event.TxHash)
	event.EventIndex = strings.TrimSpace(event.EventIndex)
	event.EventID = strings.TrimSpace(event.EventID)
	event.Contract = strings.TrimSpace(event.Contract)
	event.To = strings.TrimSpace(event.To)
	if event.EventID == "" {
		event.EventID = event.TxHash + ":" + event.EventIndex
	}
	return event
}

// SettleDirectUSDTTRC20Event atomically records a verified chain event, marks
// the TopUp successful and increments the user's quota through the existing
// TopUp CAS path. Duplicate events are harmless; a different event can never
// settle the same order after the first event wins.
func SettleDirectUSDTTRC20Event(input DirectUSDTTransferEvent) error {
	event := input.normalized()
	if event.Network == "" {
		event.Network = "TRON"
	}
	if !isDirectPaymentEventValid(event) || !validDirectUSDTNetwork(event.Network, event.Contract) {
		return ErrDirectPaymentInvalid
	}
	// Resolve the immutable provider snapshot before the settlement CAS. New
	// orders use crypto_direct; older network-specific rows keep their original
	// provider so watcher reconciliation remains backward-compatible.
	var existingTopUp TopUp
	if err := DB.Where("trade_no = ?", event.TradeNo).First(&existingTopUp).Error; err != nil {
		return err
	}
	provider := strings.ToLower(strings.TrimSpace(existingTopUp.PaymentProvider))
	if !isDirectUSDTNetworkProvider(provider) {
		return ErrDirectPaymentInvalid
	}

	var direct DirectCryptoPayment
	var quotaToAdd int
	_, applied, err := completeTopUpCASWithOptions(event.TradeNo, provider, true,
		func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
			query := tx.Where("trade_no = ?", event.TradeNo)
			if tx.Dialector.Name() != "sqlite" {
				query = query.Set("gorm:query_option", "FOR UPDATE")
			}
			if err := query.First(&direct).Error; err != nil {
				return nil, err
			}
			if direct.Status == DirectCryptoPaid {
				if direct.EventID == event.EventID {
					if direct.TxHash != event.TxHash || direct.EventIndex != event.EventIndex || direct.ObservedUnits != event.AmountUnits || direct.Address != event.To {
						return nil, ErrDirectPaymentInvalid
					}
					return nil, nil
				}
				return nil, ErrDirectPaymentAlreadySettled
			}
			if direct.Status != DirectCryptoPending && direct.Status != DirectCryptoExpired {
				return nil, ErrDirectPaymentAlreadySettled
			}
			// TronGrid reports block_timestamp in milliseconds. The blockchain event
			// time, rather than the reconciliation/current time, is the immutable
			// settlement policy: an event is eligible only inside the order window.
			if event.BlockTimestamp <= 0 || direct.CreatedAt <= 0 || direct.ExpiresAt <= direct.CreatedAt {
				return nil, ErrDirectPaymentInvalid
			}
			blockUnix := event.BlockTimestamp
			if strings.EqualFold(event.Network, "TRON") {
				blockUnix = event.BlockTimestamp / 1000
			}
			if blockUnix < direct.CreatedAt {
				return nil, ErrDirectPaymentInvalid
			}
			if blockUnix > direct.ExpiresAt {
				return nil, ErrDirectPaymentExpired
			}
			if direct.UserId != topUp.UserId || !validDirectUSDTNetwork(direct.Network, direct.Contract) || direct.Token != "USDT" || direct.Address == "" || !strings.EqualFold(direct.Network, event.Network) {
				return nil, ErrDirectPaymentInvalid
			}
			destination := direct.Address
			if direct.Destination != "" {
				destination = direct.Destination
			}
			if event.To != destination {
				return nil, ErrDirectPaymentInvalid
			}
			if event.AmountUnits != direct.ExpectedUnits {
				return nil, ErrDirectPaymentAmountMismatch
			}
			resolved, err := resolveTopUpQuotaWithDB(tx, topUp)
			if err != nil {
				return nil, err
			}
			quotaToAdd = resolved
			return nil, nil
		},
		func(tx *gorm.DB, topUp *TopUp) error {
			metadataValue, marshalErr := common.Marshal(map[string]interface{}{
				"tx_hash":         event.TxHash,
				"event_index":     event.EventIndex,
				"amount_units":    event.AmountUnits,
				"contract":        event.Contract,
				"to":              event.To,
				"confirmations":   event.Confirmations,
				"block_timestamp": event.BlockTimestamp,
			})
			if marshalErr != nil {
				return marshalErr
			}
			metadata := &PaymentMetadata{
				TradeNo:           event.TradeNo,
				PaymentProvider:   provider,
				ExternalPaymentID: event.EventID,
				Metadata:          string(metadataValue),
				CreateTime:        time.Now().Unix(),
				UpdateTime:        time.Now().Unix(),
			}
			if err := tx.Create(metadata).Error; err != nil {
				if !isUniqueConstraintError(err) {
					return err
				}
				var existing PaymentMetadata
				if lookupErr := tx.Where("payment_provider = ? AND external_payment_id = ?", provider, event.EventID).First(&existing).Error; lookupErr != nil {
					return err
				}
				if existing.TradeNo != event.TradeNo {
					return ErrDirectPaymentEventAlreadyUsed
				}
			}

			result := tx.Model(&DirectCryptoPayment{}).
				Where("id = ? AND status IN ?", direct.Id, []string{DirectCryptoPending, DirectCryptoExpired}).
				Updates(map[string]interface{}{
					"status":         DirectCryptoPaid,
					"tx_hash":        event.TxHash,
					"event_index":    event.EventIndex,
					"event_id":       event.EventID,
					"observed_units": event.AmountUnits,
					"confirmations":  event.Confirmations,
					"updated_at":     time.Now().Unix(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrDirectPaymentAlreadySettled
			}
			result = tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrTopUpUserNotFound
			}
			return creditReferralDepositReward(tx, topUp, quotaToAdd)
		})
	if err == nil {
		// completeTopUpCAS intentionally does not invoke callbacks when the
		// TopUp is already successful. Re-read the direct snapshot so a second
		// chain event can never be silently accepted for the same order.
		current, lookupErr := GetDirectCryptoPayment(event.TradeNo)
		if lookupErr != nil {
			return lookupErr
		}
		if current.Status == DirectCryptoPaid && current.EventID == event.EventID {
			return nil
		}
		if applied || current.Status == DirectCryptoPaid {
			return ErrDirectPaymentAlreadySettled
		}
		return ErrTopUpStatusInvalid
	}
	if errors.Is(err, ErrTopUpExpired) || errors.Is(err, ErrDirectPaymentExpired) {
		// The direct settlement CAS commits the paired expired states before
		// returning this terminal error. Do not perform a second transaction here:
		// a crash between two separate updates could otherwise strand the pair.
		return ErrDirectPaymentExpired
	}
	if errors.Is(err, ErrDirectPaymentAlreadySettled) {
		current, lookupErr := GetDirectCryptoPayment(event.TradeNo)
		if lookupErr == nil && current.Status == DirectCryptoPaid && current.EventID == event.EventID {
			return nil
		}
	}
	return err
}

// SettleDirectUSDTTRC20 is retained as a small compatibility wrapper for
// internal callers/tests. Production settlement uses the event form above,
// which requires the watcher-provided destination, contract and confirmation
// proof.
func SettleDirectUSDTTRC20(tradeNo, txHash string, amountUnits, confirmations uint64) error {
	payment, err := GetDirectCryptoPayment(tradeNo)
	if err != nil {
		return err
	}
	return SettleDirectUSDTTRC20Event(DirectUSDTTransferEvent{
		TradeNo: tradeNo, TxHash: txHash, EventIndex: "0", Contract: payment.Contract,
		To: payment.Address, AmountUnits: amountUnits, Confirmations: confirmations, Confirmed: true,
		BlockTimestamp: time.Now().UnixMilli(),
	})
}

func expireDirectUSDTOrder(tradeNo string) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// Keep the lock order identical to completeTopUpCAS (TopUp first,
		// direct-payment row second) to avoid a cross-order deadlock when an
		// expired event races with a valid settlement.
		var topUp TopUp
		topUpQuery := tx.Where("trade_no = ? AND payment_provider IN ? AND status = ?", tradeNo, []string{DirectCryptoProvider, DirectUSDTTRC20Provider, operation_setting.DirectUSDTTONPaymentMethod, operation_setting.DirectUSDTSolanaPaymentMethod}, common.TopUpStatusPending)
		if tx.Dialector.Name() != "sqlite" {
			topUpQuery = topUpQuery.Set("gorm:query_option", "FOR UPDATE")
		}
		if err := topUpQuery.First(&topUp).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().Unix()
		if err := expireDirectPaymentTx(tx, tradeNo, now); err != nil {
			return err
		}
		if topUp.Id == 0 {
			return nil
		}
		return tx.Model(&TopUp{}).
			Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).
			Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now}).Error
	})
}

// expireDirectPaymentTx closes only a still-pending direct snapshot. It is
// intentionally called from the same transaction as the paired TopUp update
// so expiry cannot be observed half-applied after a crash.
func expireDirectPaymentTx(tx *gorm.DB, tradeNo string, now int64) error {
	return tx.Model(&DirectCryptoPayment{}).
		Where("trade_no = ? AND status = ?", strings.TrimSpace(tradeNo), DirectCryptoPending).
		Updates(map[string]interface{}{"status": DirectCryptoExpired, "updated_at": now}).Error
}

// GetDirectCryptoPaymentStatus loads a direct order and, when its immutable
// deadline has passed, expires both the direct snapshot and its TopUp row in
// one transaction.  This keeps the public status endpoint from observing a
// pending order on one table and an expired order on the other.
func GetDirectCryptoPaymentStatus(tradeNo string, now int64) (*DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payment DirectCryptoPayment
	err := DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		// Load immutable direct snapshot first. Existing network-specific provider
		// IDs are accepted for reconciliation, while new orders use crypto_direct.
		var snapshot DirectCryptoPayment
		if err := tx.Where("trade_no = ?", strings.TrimSpace(tradeNo)).First(&snapshot).Error; err != nil {
			return err
		}
		topUpQuery := tx.Where("trade_no = ? AND payment_provider IN ?", strings.TrimSpace(tradeNo), []string{DirectCryptoProvider, DirectUSDTTRC20Provider, operation_setting.DirectUSDTTONPaymentMethod, operation_setting.DirectUSDTSolanaPaymentMethod})
		if tx.Dialector.Name() != "sqlite" {
			topUpQuery = topUpQuery.Set("gorm:query_option", "FOR UPDATE")
		}
		topUpErr := topUpQuery.First(&topUp).Error
		if topUpErr != nil && !errors.Is(topUpErr, gorm.ErrRecordNotFound) {
			return topUpErr
		}

		paymentQuery := tx.Where("trade_no = ?", strings.TrimSpace(tradeNo))
		if tx.Dialector.Name() != "sqlite" {
			paymentQuery = paymentQuery.Set("gorm:query_option", "FOR UPDATE")
		}
		if err := paymentQuery.First(&payment).Error; err != nil {
			return err
		}

		if payment.Status == DirectCryptoPaid {
			// A successful direct payment is terminal.  Never rewrite it based on
			// a mutable PayMethods TTL or a stale TopUp row.
			return nil
		}
		if payment.Status == DirectCryptoPending && topUpErr == nil &&
			(topUp.Status == common.TopUpStatusExpired || topUp.Status == common.TopUpStatusFailed) {
			targetStatus := DirectCryptoExpired
			if topUp.Status == common.TopUpStatusFailed {
				targetStatus = DirectCryptoFailed
			}
			result := tx.Model(&DirectCryptoPayment{}).
				Where("id = ? AND status = ?", payment.Id, DirectCryptoPending).
				Updates(map[string]interface{}{"status": targetStatus, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				payment.Status = targetStatus
				payment.UpdatedAt = now
			}
			return nil
		}
		if payment.Status != DirectCryptoPending {
			// If an earlier sweep marked the direct snapshot expired, finish the
			// paired TopUp transition if it is still pending.
			if payment.Status == DirectCryptoExpired && topUpErr == nil && topUp.Status == common.TopUpStatusPending {
				result := tx.Model(&TopUp{}).
					Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).
					Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now})
				return result.Error
			}
			return nil
		}
		// The direct snapshot is authoritative; using the current PayMethods
		// TTL here could expire an order earlier or later after an operator edits
		// configuration.
		directExpired := payment.ExpiresAt > 0 && now >= payment.ExpiresAt
		if !directExpired {
			return nil
		}

		result := tx.Model(&DirectCryptoPayment{}).
			Where("id = ? AND status = ?", payment.Id, DirectCryptoPending).
			Updates(map[string]interface{}{"status": DirectCryptoExpired, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			payment.Status = DirectCryptoExpired
			payment.UpdatedAt = now
		}
		if topUpErr == nil {
			result = tx.Model(&TopUp{}).
				Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).
				Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now})
			if result.Error != nil {
				return result.Error
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func DirectUSDTEventID(txHash, eventIndex string) string {
	return strings.TrimSpace(txHash) + ":" + strings.TrimSpace(eventIndex)
}

func DirectUSDTAmountString(units uint64) string {
	whole := units / 1_000_000
	frac := units % 1_000_000
	return strconv.FormatUint(whole, 10) + "." + fmt.Sprintf("%06d", frac)
}

// MigrateDirectCryptoPaymentIndexes removes the unique TxHash index from the
// early draft schema. A TRON transaction may contain multiple Transfer events;
// event identity is tx hash plus event index, while TxHash itself is not
// unique. The call is idempotent on SQLite, MySQL and PostgreSQL.
func MigrateDirectCryptoPaymentIndexes(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	for _, indexName := range []string{"idx_direct_crypto_payments_tx_hash", "idx_direct_crypto_payment_tx_hash"} {
		if !db.Migrator().HasIndex(&DirectCryptoPayment{}, indexName) {
			continue
		}
		// Keep the explicitly named non-unique index introduced by the current
		// schema; only drop the historical auto-named unique index.
		if indexName == "idx_direct_crypto_payment_tx_hash" {
			continue
		}
		if err := db.Migrator().DropIndex(&DirectCryptoPayment{}, indexName); err != nil {
			return err
		}
	}
	return nil
}
