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
	"gorm.io/gorm/clause"
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
	DirectUSDTLegacyMaxPendingSeconds int64 = int64(operation_setting.MaxDirectUSDTTRC20PendingTTL / time.Second)
	DirectUSDTMaxPendingSeconds       int64 = int64(24 * time.Hour / time.Second)
	DirectUSDTNewMaxPendingSeconds    int64 = DirectUSDTMaxPendingSeconds

	DirectCryptoPending   = "pending"
	DirectCryptoPaid      = "paid"
	DirectCryptoExpired   = "expired"
	DirectCryptoFailed    = "failed"
	DirectCryptoCancelled = "cancelled"
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
	ErrDirectPaymentDuplicatePending = errors.New("a pending direct USDT payment already exists for this amount")
	errDirectPaymentWalletOccupied   = errors.New("direct USDT wallet already has this base amount")
	errDirectPaymentSuffixOccupied   = errors.New("direct USDT suffix is already reserved")
)

// DirectCryptoPayment is an immutable amount reservation plus the observed
// chain event. ExpectedUnits is in USDT's six-decimal smallest unit and is
// globally reserved for the lifetime of the database: an expired order is
// intentionally never reused because a late transfer must not settle a newer
// order. Wallet pools still distribute equal base amounts by choosing a
// different suffix for each receiving wallet.
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
	WalletKey      string `gorm:"type:varchar(255)" json:"wallet_key,omitempty"`
	// Keep the historical global uniqueness guard. A different wallet can still
	// reserve the same base amount by receiving a different suffix; preserving
	// this index also keeps an older binary safe during rollback.
	ExpectedUnits uint64 `gorm:"not null;uniqueIndex:idx_direct_crypto_payment_expected_units" json:"exact_amount_units"`
	BaseUnits     uint64 `gorm:"not null" json:"base_amount_units"`
	SuffixUnits   uint32 `gorm:"not null" json:"suffix_units"`
	// These columns stay nullable for rollback compatibility: an older binary
	// does not mention them in INSERT statements. Go reads NULL as false, while
	// the rollout migration backfills existing rows to false.
	RoundToCents        bool   `json:"round_to_cents"`
	RoundPolicyCaptured bool   `json:"round_policy_captured"`
	Status              string `gorm:"type:varchar(16);index;not null" json:"status"`
	TxHash              string `gorm:"type:varchar(128);index:idx_direct_crypto_payment_tx_hash" json:"tx_hash,omitempty"`
	EventIndex          string `gorm:"type:varchar(64)" json:"event_index,omitempty"`
	EventID             string `gorm:"type:varchar(255);index" json:"event_id,omitempty"`
	ObservedUnits       uint64 `json:"observed_units,omitempty"`
	Confirmations       uint64 `json:"confirmations,omitempty"`
	ExpiresAt           int64  `gorm:"index;not null" json:"expires_at"`
	CreatedAt           int64  `gorm:"index;not null" json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// DirectUSDTTransferEvent is produced by a chain watcher. Watchers provide
// confirmation/finality evidence, while settlement repeats all security checks
// against the immutable order snapshot before crediting.
type DirectUSDTTransferEvent struct {
	TradeNo        string
	TxHash         string
	EventIndex     string
	EventID        string
	Contract       string
	Source         string
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
	err := DB.Where("expected_units = ? AND address = ? AND (status = ? OR status = ?) AND expires_at >= ?", expectedUnits, strings.TrimSpace(address), DirectCryptoPending, DirectCryptoExpired, cutoff).
		Find(&payments).Error
	return payments, err
}

// GetCancelledDirectUSDTPayments returns cancelled snapshots that are still
// inside the reconciliation overlap. They are never eligible for settlement,
// but the watcher must inspect them so a later transfer is retained as a
// non-crediting audit event instead of disappearing from reconciliation.
func GetCancelledDirectUSDTPayments(expectedUnits uint64, address string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("expected_units = ? AND address = ? AND status = ? AND expires_at >= ?", expectedUnits, strings.TrimSpace(address), DirectCryptoCancelled, cutoff).
		Find(&payments).Error
	return payments, err
}

func GetPendingDirectUSDTNetworkPayments(network string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("network = ? AND (status = ? OR status = ?) AND expires_at >= ?", strings.ToUpper(strings.TrimSpace(network)), DirectCryptoPending, DirectCryptoExpired, cutoff).Find(&payments).Error
	return payments, err
}

// GetDirectUSDTNetworkReconciliationPayments returns every immutable snapshot
// that is still inside the watcher overlap. It is intentionally broader than
// the exact-amount lookup used for settlement: reconciliation must also retain
// underpayments, overpayments and transfers with no matching invoice.
func GetDirectUSDTNetworkReconciliationPayments(network string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("network = ? AND (status = ? OR status = ? OR status = ?) AND expires_at >= ?", strings.ToUpper(strings.TrimSpace(network)), DirectCryptoPending, DirectCryptoExpired, DirectCryptoCancelled, cutoff).Find(&payments).Error
	return payments, err
}

// GetCancelledDirectUSDTNetworkPayments is the multichain counterpart of
// GetCancelledDirectUSDTPayments. Cancelled rows are read only for late-event
// audit; SettleDirectUSDTTRC20Event always refuses to credit them.
func GetCancelledDirectUSDTNetworkPayments(network string) ([]DirectCryptoPayment, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var payments []DirectCryptoPayment
	cutoff := time.Now().Unix() - DirectUSDTMaxPendingSeconds
	err := DB.Where("network = ? AND status = ? AND expires_at >= ?", strings.ToUpper(strings.TrimSpace(network)), DirectCryptoCancelled, cutoff).Find(&payments).Error
	return payments, err
}

// GetActivePendingDirectUSDTPaymentAddresses returns every receiving address
// captured by an invoice still inside the reconciliation window, including
// cancelled snapshots kept for late-event audit. The address is part of the
// immutable order snapshot; using only the current setting would orphan orders
// created before an operator rotated the wallet.
func GetActivePendingDirectUSDTPaymentAddresses(now int64) ([]string, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var addresses []string
	cutoff := now - DirectUSDTMaxPendingSeconds
	// Pending, expired and cancelled snapshots are bounded by the watcher
	// overlap so pre-expiry events remain discoverable after wallet rotation.
	err := DB.Model(&DirectCryptoPayment{}).
		Where("address <> '' AND (status = ? OR status = ? OR status = ?) AND expires_at >= ?", DirectCryptoPending, DirectCryptoExpired, DirectCryptoCancelled, cutoff).
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
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
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
	if wallets, err := setting.USDTReceivingWalletsForNetwork(network); err == nil && len(wallets) > 0 {
		return wallets[0].Address
	}
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
	if strings.TrimSpace(setting.USDTReceivingWallets) != "" {
		wallets, err := directUSDTWalletCandidates(network)
		return err == nil && len(wallets) > 0
	}
	address := directUSDTReceivingAddress(network)
	if err := ValidateDirectUSDTNetworkConfig(network, address); err != nil {
		return false
	}
	if strings.EqualFold(network, "SOLANA") {
		return setting.ValidateSolanaTokenAccount(address, setting.USDTSolanaReceivingTokenAccount) == nil
	}
	return true
}

func directUSDTWalletCandidates(network string) ([]setting.USDTReceivingWallet, error) {
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "TON":
		if err := setting.ValidateUSDTProviderEndpoint("TON", setting.USDTTONAPIBaseURL); err != nil {
			return nil, err
		}
	case "SOLANA":
		if err := setting.ValidateUSDTProviderEndpoint("SOLANA", setting.USDTSolanaRPCURL); err != nil {
			return nil, err
		}
	}
	wallets, err := setting.USDTReceivingWalletsForNetwork(network)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(setting.USDTReceivingWallets) == "" || isEmptyUSDTWalletPool(setting.USDTReceivingWallets) {
		address := directUSDTReceivingAddress(network)
		if err := ValidateDirectUSDTNetworkConfig(network, address); err != nil {
			return nil, err
		}
		wallet := setting.USDTReceivingWallet{Key: directUSDTWalletKey(network, address), Network: strings.ToUpper(network), Address: address, Owner: address, Destination: address}
		if strings.EqualFold(network, "SOLANA") {
			wallet.Destination = setting.USDTSolanaReceivingTokenAccount
		}
		return []setting.USDTReceivingWallet{wallet}, nil
	}
	for i := range wallets {
		w := &wallets[i]
		if w.Network == "TON" {
			canonical, err := setting.CanonicalTONAddress(w.Address)
			if err != nil {
				return nil, err
			}
			w.Address = canonical
			if w.Owner == "" {
				w.Owner = canonical
			} else if owner, ownerErr := setting.CanonicalTONAddress(w.Owner); ownerErr != nil {
				return nil, ownerErr
			} else {
				w.Owner = owner
			}
			if w.Destination == "" {
				w.Destination = canonical
			} else if destination, destinationErr := setting.CanonicalTONAddress(w.Destination); destinationErr != nil {
				return nil, destinationErr
			} else {
				w.Destination = destination
			}
		}
		if w.Owner == "" {
			w.Owner = w.Address
		}
		if w.Destination == "" {
			w.Destination = w.Address
		}
		if w.Network == "SOLANA" {
			if err := setting.ValidateSolanaAddress(w.Address); err != nil {
				return nil, err
			}
			if err := setting.ValidateSolanaAddress(w.Destination); err != nil {
				return nil, err
			}
			if strings.TrimSpace(setting.USDTSolanaAPIKey) == "" {
				return nil, errors.New("Solana RPC API key is required when USDT Solana is enabled")
			}
			if err := setting.ValidateSolanaTokenAccount(w.Owner, w.Destination); err != nil {
				return nil, err
			}
		} else if err := setting.ValidateDirectUSDTMultiChainConfig(w.Network, w.Address, map[string]string{"TRON": setting.USDTTRC20APIKey, "TON": setting.USDTTONAPIKey}[w.Network]); err != nil {
			return nil, err
		}
	}
	return wallets, nil
}

func isEmptyUSDTWalletPool(value string) bool {
	wallets, err := setting.ParseUSDTReceivingWallets(value)
	return err == nil && len(wallets) == 0
}

func directUSDTWalletKey(network, address string) string {
	return strings.ToUpper(strings.TrimSpace(network)) + ":" + strings.TrimSpace(address)
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
			ttl = time.Duration(min(minutes, int(DirectUSDTNewMaxPendingSeconds/60))) * time.Minute
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
	payment.Network = strings.ToUpper(strings.TrimSpace(payment.Network))
	provider := DirectCryptoProvider
	if !IsDirectUSDTNetworkMethodConfigured(provider) || !DirectUSDTNetworkIsReady(payment.Network) {
		return ErrDirectPaymentDisabled
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
	wallets, err := directUSDTWalletCandidates(payment.Network)
	if err != nil {
		return err
	}
	if len(wallets) == 0 {
		return ErrDirectPaymentDisabled
	}
	if strings.TrimSpace(setting.USDTReceivingWallets) == "" && !sameDirectUSDTAddress(payment.Network, payment.Address, wallets[0].Address) {
		return errors.New("direct USDT TRC20 order address does not match configured receiving address")
	}
	roundToCents := setting.USDTTRC20RoundToCents
	if payment.RoundPolicyCaptured {
		roundToCents = payment.RoundToCents
	}
	payment.RoundToCents = roundToCents
	topUp.PaymentRoundToCents = roundToCents
	payment.BaseUnits = NormalizeDirectUSDTBaseUnits(payment.BaseUnits, roundToCents)
	if payment.BaseUnits < DirectUSDTMinBaseUnits {
		return errors.New("invalid direct USDT TRC20 order")
	}
	// Keep the immutable TopUp accounting snapshot aligned with the amount that
	// will actually be reserved. This matters for callers that construct orders
	// directly instead of going through the controller's cents normalization.
	normalizedBaseAmount := float64(payment.BaseUnits) / 1_000_000
	topUp.RequestedAmount = normalizedBaseAmount
	topUp.PaymentBaseAmount = normalizedBaseAmount
	// Accept the legacy 48-hour input envelope long enough to capture the
	// persisted policy below.  The captured policy is then clamped to the
	// current 24-hour maximum before the row is written; rejecting 24–48h here
	// would make valid legacy callers fail before that normalization happens.
	if payment.ExpiresAt <= payment.CreatedAt || payment.CreatedAt <= 0 ||
		payment.ExpiresAt-payment.CreatedAt > DirectUSDTLegacyMaxPendingSeconds {
		return errors.New("direct USDT TRC20 order has invalid expiry")
	}
	if err := CaptureDirectCryptoPolicy(topUp); err != nil {
		return err
	}
	payment.RoundPolicyCaptured = true
	payment.ExpiresAt = payment.CreatedAt + topUp.PaymentPendingTTLSeconds
	if payment.ExpiresAt <= payment.CreatedAt || payment.CreatedAt <= 0 ||
		payment.ExpiresAt-payment.CreatedAt > DirectUSDTNewMaxPendingSeconds {
		return errors.New("direct USDT TRC20 order has invalid expiry")
	}
	if baseUSD := float64(payment.BaseUnits) / 1_000_000; baseUSD < topUp.PaymentMinimumAmount {
		return errors.New("direct USDT amount is below the configured method minimum")
	}
	// A direct order can be created through more than one entry point. Expire
	// stale local rows before duplicate and wallet-allocation checks so an
	// unrun background sweep cannot keep a wallet or amount reserved forever.
	if err := ExpireStalePendingTopUps(topUp.UserId); err != nil {
		return err
	}
	suffixMinUnits, suffixMaxUnits, suffixStepUnits, err := DirectUSDTAmountSuffixRange(roundToCents)
	if err != nil {
		return err
	}
	suffixRangeSize := int64((suffixMaxUnits-suffixMinUnits)/suffixStepUnits + 1)

	const maxAttempts = 32
	for walletIndex, wallet := range wallets {
		if walletIndex > 0 {
			payment.Id = 0
			topUp.Id = 0
		}
		for attempt := 0; attempt < maxAttempts; attempt++ {
			suffix, err := randomDirectUSDTTRC20SuffixForPolicy(roundToCents)
			if err != nil {
				return err
			}
			payment.Id = 0
			topUp.Id = 0
			payment.SuffixUnits = suffix
			payment.ExpectedUnits = payment.BaseUnits + uint64(suffix)
			payment.WalletKey = wallet.Key
			payment.Address = wallet.Address
			payment.ReceivingOwner = wallet.Owner
			payment.Destination = wallet.Destination
			if payment.ReceivingOwner == "" {
				payment.ReceivingOwner = payment.Address
			}
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
				if err := lockDirectUSDTWalletAllocation(tx); err != nil {
					return err
				}
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
				var existing DirectCryptoPayment
				duplicateQuery := tx.Where("user_id = ? AND base_units = ? AND (status = ? OR (status = ? AND expires_at >= ?))", topUp.UserId, payment.BaseUnits, DirectCryptoPending, DirectCryptoExpired, payment.CreatedAt)
				if err := duplicateQuery.First(&existing).Error; err == nil {
					return ErrDirectPaymentDuplicatePending
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if strings.TrimSpace(setting.USDTReceivingWallets) != "" {
					var walletAmountCount int64
					if err := tx.Model(&DirectCryptoPayment{}).
						Where("(wallet_key = ? OR (network = ? AND address = ?)) AND base_units = ? AND (status = ? OR (status = ? AND expires_at >= ?))", payment.WalletKey, payment.Network, payment.Address, payment.BaseUnits, DirectCryptoPending, DirectCryptoExpired, payment.CreatedAt).
						Count(&walletAmountCount).Error; err != nil {
						return err
					}
					if walletAmountCount > 0 {
						return errDirectPaymentWalletOccupied
					}
				}
				var usedSuffixes []uint32
				// Rows created before WalletKey was introduced are still part of the
				// matching wallet's permanent suffix quarantine until migration runs.
				// Match those rows by immutable network/address even when the current
				// wallet pool assigns an arbitrary custom key.
				usedSuffixQuery := tx.Model(&DirectCryptoPayment{}).
					Where("(wallet_key = ? OR (network = ? AND address = ?)) AND base_units = ? AND suffix_units >= ? AND suffix_units <= ?", payment.WalletKey, payment.Network, payment.Address, payment.BaseUnits, suffixMinUnits, suffixMaxUnits)
				if err := usedSuffixQuery.Pluck("suffix_units", &usedSuffixes).Error; err != nil {
					return err
				}
				// Keep the historical global exact-amount uniqueness constraint
				// active so an older binary can safely run against this schema.
				// Different wallets still accept the same base amount; they receive
				// different suffixes. Skip already-used exact amounts before INSERT
				// instead of relying on a late unique-constraint retry.
				var usedExpectedUnits []uint64
				amountMin := payment.BaseUnits + uint64(suffixMinUnits)
				amountMax := payment.BaseUnits + uint64(suffixMaxUnits)
				if amountMax < payment.BaseUnits {
					return errors.New("direct USDT TRC20 amount overflow")
				}
				if err := tx.Model(&DirectCryptoPayment{}).
					Where("expected_units >= ? AND expected_units <= ?", amountMin, amountMax).
					Pluck("expected_units", &usedExpectedUnits).Error; err != nil {
					return err
				}
				usedSet := make(map[uint32]struct{}, len(usedSuffixes))
				for _, usedSuffix := range usedSuffixes {
					if int(usedSuffix) >= suffixMinUnits && int(usedSuffix) <= suffixMaxUnits && (int(usedSuffix)-suffixMinUnits)%suffixStepUnits == 0 {
						usedSet[usedSuffix] = struct{}{}
					}
				}
				usedAmountSet := make(map[uint64]struct{}, len(usedExpectedUnits))
				for _, usedAmount := range usedExpectedUnits {
					usedAmountSet[usedAmount] = struct{}{}
				}
				// Every configured suffix is permanently quarantined after use so a
				// late transfer can never be mistaken for a newer order.
				if int64(len(usedSet)) >= suffixRangeSize {
					return errDirectPaymentWalletOccupied
				}
				_, walletSuffixOccupied := usedSet[payment.SuffixUnits]
				_, amountOccupied := usedAmountSet[payment.ExpectedUnits]
				if walletSuffixOccupied || amountOccupied {
					// A random collision must not make a wallet look exhausted. The
					// allocation lock is held, so choose the first free suffix from the
					// same immutable range and continue with this transaction.
					found := false
					for candidate := suffixMinUnits; candidate <= suffixMaxUnits; candidate += suffixStepUnits {
						candidateAmount := payment.BaseUnits + uint64(candidate)
						if _, occupied := usedSet[uint32(candidate)]; occupied {
							continue
						}
						if _, occupied := usedAmountSet[candidateAmount]; occupied {
							continue
						}
						payment.SuffixUnits = uint32(candidate)
						payment.ExpectedUnits = payment.BaseUnits + uint64(payment.SuffixUnits)
						expectedAmount, parseErr := strconv.ParseFloat(DirectUSDTAmountString(payment.ExpectedUnits), 64)
						if parseErr != nil {
							return parseErr
						}
						topUp.PaymentChargedAmount = expectedAmount
						topUp.Money = expectedAmount
						found = true
						break
					}
					if !found {
						return errDirectPaymentWalletOccupied
					}
				}
				if err := tx.Create(topUp).Error; err != nil {
					return err
				}
				return tx.Create(payment).Error
			})
			if err == nil {
				return nil
			}
			if errors.Is(err, ErrDirectPaymentDuplicatePending) {
				return err
			}
			if errors.Is(err, errDirectPaymentWalletOccupied) {
				continue
			}
			if errors.Is(err, errDirectPaymentSuffixOccupied) {
				continue
			}
			if errors.Is(err, ErrDirectPaymentLimitExceeded) {
				return err
			}
			if errors.Is(err, ErrDirectPaymentAmountExhausted) {
				continue
			}
			if isSQLiteBusyError(err) || isUniqueConstraintError(err) {
				continue
			}
			return err
		}
	}
	return ErrDirectPaymentAmountExhausted
}

const (
	directUSDTWalletAllocationLockKey = "DirectUSDTWalletAllocationLock"
	directUSDTEventDecisionLockKey    = "DirectUSDTEventDecisionLock"
)

// lockDirectUSDTWalletAllocation serializes wallet/base assignment across
// application instances. The options table is present in every migrated
// production database; tests and very old uninitialized databases may not have
// it yet, in which case the caller's existing uniqueness checks remain active.
func lockDirectUSDTWalletAllocation(tx *gorm.DB) error {
	return lockDirectUSDTOption(tx, directUSDTWalletAllocationLockKey)
}

func lockDirectUSDTEventDecision(tx *gorm.DB) error {
	return lockDirectUSDTOption(tx, directUSDTEventDecisionLockKey)
}

func lockDirectUSDTOption(tx *gorm.DB, key string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	var lock Option
	query := tx.Where("key = ?", key)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&lock).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		createErr := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: key, Value: "1"}).Error
		if createErr != nil && !isUniqueConstraintError(createErr) {
			if isMissingOptionsTableError(createErr) {
				return nil
			}
			return createErr
		}
		lockedQuery := tx.Where("key = ?", key)
		if tx.Dialector.Name() != "sqlite" {
			lockedQuery = lockedQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		return lockedQuery.First(&lock).Error
	}
	if err != nil && isMissingOptionsTableError(err) {
		return nil
	}
	return err
}

func lockDirectUSDTUser(tx *gorm.DB, userID int) error {
	query := tx.Select("id").Where("id = ?", userID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
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
	return randomDirectUSDTTRC20SuffixForPolicy(false)
}

func randomDirectUSDTTRC20SuffixForPolicy(roundToCents bool) (uint32, error) {
	// crypto/rand is used rather than math/rand so concurrent workers cannot
	// predict or intentionally collide with a user's amount reservation. Use
	// rejection sampling instead of modulo alone so every configured suffix is
	// equally likely even when the range does not divide 2^16.
	minUnits, maxUnits, stepUnits, err := DirectUSDTAmountSuffixRange(roundToCents)
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
	return strings.TrimSpace(event.TradeNo) != "" && isDirectPaymentChainEventValid(event)
}

func isDirectPaymentChainEventValid(event DirectUSDTTransferEvent) bool {
	minConfirmations := setting.USDTTRC20MinConfirmations
	if !strings.EqualFold(event.Network, "TRON") {
		minConfirmations = 0
	}
	expectedEventID := DirectUSDTNetworkEventID(event.Network, event.TxHash, event.EventIndex)
	legacyEventID := DirectUSDTEventID(event.TxHash, event.EventIndex)
	return strings.TrimSpace(event.TxHash) != "" &&
		strings.TrimSpace(event.EventIndex) != "" &&
		strings.TrimSpace(event.EventID) != "" &&
		(event.EventID == expectedEventID || (strings.EqualFold(event.Network, "TRON") && event.EventID == legacyEventID)) &&
		event.AmountUnits > 0 && event.Confirmed &&
		(minConfirmations <= 0 || event.Confirmations >= uint64(minConfirmations))
}

func (event DirectUSDTTransferEvent) normalized() DirectUSDTTransferEvent {
	event.TradeNo = strings.TrimSpace(event.TradeNo)
	event.Network = strings.ToUpper(strings.TrimSpace(event.Network))
	event.TxHash = strings.TrimSpace(event.TxHash)
	event.EventIndex = strings.TrimSpace(event.EventIndex)
	event.EventID = strings.TrimSpace(event.EventID)
	event.Contract = strings.TrimSpace(event.Contract)
	event.Source = strings.TrimSpace(event.Source)
	event.To = strings.TrimSpace(event.To)
	if event.EventID == "" {
		event.EventID = DirectUSDTNetworkEventID(event.Network, event.TxHash, event.EventIndex)
	}
	return event
}

// validateDirectUSDTEventForPayment verifies a chain event against the
// immutable invoice snapshot. It is shared by crediting and late-event audit
// paths so cancelled invoices cannot retain unrelated transfers as evidence.
func validateDirectUSDTEventForPayment(event DirectUSDTTransferEvent, payment DirectCryptoPayment) error {
	return validateDirectUSDTEventForPaymentWithExpiry(event, payment, false)
}

// allowAfterExpiry is reserved for cancelled snapshots: their verified chain
// events are retained as non-crediting audit evidence even when sent after the
// invoice deadline.
func validateDirectUSDTEventForPaymentWithExpiry(event DirectUSDTTransferEvent, payment DirectCryptoPayment, allowAfterExpiry bool) error {
	if event.BlockTimestamp <= 0 || payment.CreatedAt <= 0 || payment.ExpiresAt <= payment.CreatedAt ||
		payment.Token != "USDT" || payment.Address == "" ||
		!validDirectUSDTNetwork(payment.Network, payment.Contract) ||
		!strings.EqualFold(payment.Network, event.Network) ||
		!strings.EqualFold(payment.Contract, event.Contract) {
		return ErrDirectPaymentInvalid
	}
	blockUnix := event.BlockTimestamp
	if strings.EqualFold(event.Network, "TRON") {
		blockUnix = event.BlockTimestamp / 1000
	}
	if blockUnix < payment.CreatedAt {
		return ErrDirectPaymentInvalid
	}
	// A watcher may discover a transfer later than it was mined, but a chain
	// event cannot be mined in the future. Keep this check in settlement as
	// well as in each watcher so an exported/internal caller cannot credit an
	// invoice from a fabricated future timestamp.
	if blockUnix > time.Now().Unix() {
		return ErrDirectPaymentInvalid
	}
	if !allowAfterExpiry && blockUnix >= payment.ExpiresAt {
		return ErrDirectPaymentExpired
	}
	destination := directUSDTEventDestination(payment)
	if !sameDirectUSDTAddress(payment.Network, event.To, destination) {
		return ErrDirectPaymentInvalid
	}
	if event.AmountUnits != payment.ExpectedUnits {
		return ErrDirectPaymentAmountMismatch
	}
	return nil
}

func directUSDTEventDestination(payment DirectCryptoPayment) string {
	if payment.Destination != "" {
		return payment.Destination
	}
	return payment.Address
}

// SettleDirectUSDTTRC20Event atomically records a verified chain event, marks
// the TopUp successful and increments the user's quota through the existing
// TopUp CAS path. Duplicate events are harmless; a different event can never
// settle the same order after the first event wins.
func SettleDirectUSDTTRC20Event(input DirectUSDTTransferEvent) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(input.Network) == "" {
		input.Network = "TRON"
	}
	event := input.normalized()
	if !isDirectPaymentEventValid(event) || !validDirectUSDTNetwork(event.Network, event.Contract) {
		return ErrDirectPaymentInvalid
	}
	// A verified orphan/mismatch is permanently reserved for manual review. Do
	// not let a later invoice claim the same chain event after a watcher retry or
	// a user creates a coincidentally matching amount.
	var reconciliation DirectCryptoReconciliation
	if err := DB.Where("network = ? AND event_id = ?", event.Network, event.EventID).First(&reconciliation).Error; err == nil {
		return ErrDirectPaymentEventAlreadyUsed
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
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
	var directSnapshot DirectCryptoPayment
	if err := DB.Where("trade_no = ?", event.TradeNo).First(&directSnapshot).Error; err == nil {
		if directSnapshot.Status == DirectCryptoCancelled {
			if err := validateDirectUSDTEventForPaymentWithExpiry(event, directSnapshot, true); err != nil {
				return err
			}
			return recordLateDirectUSDTEvent(event, provider)
		}
		if directSnapshot.Status == DirectCryptoExpired {
			// An event mined before the immutable deadline may still complete an
			// expired local snapshot. Only events at/after the deadline become a
			// non-crediting late-review item.
			if err := validateDirectUSDTEventForPayment(event, directSnapshot); err != nil {
				if !errors.Is(err, ErrDirectPaymentExpired) {
					return err
				}
				if lateErr := validateDirectUSDTEventForPaymentWithExpiry(event, directSnapshot, true); lateErr != nil {
					return lateErr
				}
				return recordLateDirectUSDTEvent(event, provider)
			}
		}
	}

	var direct DirectCryptoPayment
	var quotaToAdd int
	_, applied, err := completeTopUpCASWithOptions(event.TradeNo, provider, true,
		func(tx *gorm.DB, topUp *TopUp) (map[string]interface{}, error) {
			if err := lockDirectUSDTEventDecision(tx); err != nil {
				return nil, err
			}
			var reconciliation DirectCryptoReconciliation
			if err := tx.Where("network = ? AND event_id = ?", event.Network, event.EventID).First(&reconciliation).Error; err == nil {
				return nil, ErrDirectPaymentEventAlreadyUsed
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			query := tx.Where("trade_no = ?", event.TradeNo)
			if tx.Dialector.Name() != "sqlite" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := query.First(&direct).Error; err != nil {
				return nil, err
			}
			if direct.Status == DirectCryptoPaid {
				if direct.EventID == event.EventID {
					if err := validateDirectUSDTEventForPayment(event, direct); err != nil {
						return nil, err
					}
					if direct.TxHash != event.TxHash || direct.EventIndex != event.EventIndex {
						return nil, ErrDirectPaymentInvalid
					}
					return nil, nil
				}
				return nil, ErrDirectPaymentAlreadySettled
			}
			if direct.Status != DirectCryptoPending && direct.Status != DirectCryptoExpired {
				return nil, ErrDirectPaymentAlreadySettled
			}
			if direct.UserId != topUp.UserId {
				return nil, ErrDirectPaymentInvalid
			}
			if err := validateDirectUSDTEventForPayment(event, direct); err != nil {
				return nil, err
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
				"source":          event.Source,
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
			if validateErr := validateDirectUSDTEventForPayment(event, *current); validateErr != nil {
				return validateErr
			}
			if current.TxHash != event.TxHash || current.EventIndex != event.EventIndex {
				return ErrDirectPaymentInvalid
			}
			return nil
		}
		if applied || current.Status == DirectCryptoPaid {
			return ErrDirectPaymentAlreadySettled
		}
		return ErrTopUpStatusInvalid
	}
	if errors.Is(err, ErrTopUpExpired) || errors.Is(err, ErrDirectPaymentExpired) {
		// The direct settlement CAS commits the paired expired states before
		// returning this terminal error. Keep the verified chain event as a
		// non-crediting audit item; do not perform a second state transition here.
		current, lookupErr := GetDirectCryptoPayment(event.TradeNo)
		if lookupErr == nil && current.Status == DirectCryptoExpired {
			if validateErr := validateDirectUSDTEventForPaymentWithExpiry(event, *current, true); validateErr != nil {
				return validateErr
			}
			return recordLateDirectUSDTEvent(event, provider)
		}
		return ErrDirectPaymentExpired
	}
	if errors.Is(err, ErrDirectPaymentAlreadySettled) {
		current, lookupErr := GetDirectCryptoPayment(event.TradeNo)
		if lookupErr == nil && current.Status == DirectCryptoPaid && current.EventID == event.EventID {
			if validateErr := validateDirectUSDTEventForPayment(event, *current); validateErr != nil {
				return validateErr
			}
			if current.TxHash == event.TxHash && current.EventIndex == event.EventIndex {
				return nil
			}
			return ErrDirectPaymentInvalid
		}
	}
	// Cancellation may commit after the initial snapshot check but before the
	// settlement CAS. Preserve the verified chain event as late audit data in
	// that race instead of returning a status error with no evidence.
	if errors.Is(err, ErrTopUpStatusInvalid) || errors.Is(err, ErrDirectPaymentAlreadySettled) {
		current, lookupErr := GetDirectCryptoPayment(event.TradeNo)
		if lookupErr == nil && current.Status == DirectCryptoCancelled {
			if validateErr := validateDirectUSDTEventForPaymentWithExpiry(event, *current, true); validateErr != nil {
				return validateErr
			}
			return recordLateDirectUSDTEvent(event, provider)
		}
		if lookupErr == nil && current.Status == DirectCryptoExpired {
			if validateErr := validateDirectUSDTEventForPaymentWithExpiry(event, *current, true); validateErr != nil {
				return validateErr
			}
			return recordLateDirectUSDTEvent(event, provider)
		}
	}
	return err
}

func recordLateDirectUSDTEvent(event DirectUSDTTransferEvent, provider string) error {
	eventMetadata := map[string]interface{}{
		"event_id":        event.EventID,
		"tx_hash":         event.TxHash,
		"event_index":     event.EventIndex,
		"amount_units":    event.AmountUnits,
		"contract":        event.Contract,
		"source":          event.Source,
		"to":              event.To,
		"block_timestamp": event.BlockTimestamp,
		"late":            true,
		"credited":        false,
	}
	metadataValue, err := common.Marshal(eventMetadata)
	if err != nil {
		return err
	}
	metadata := &PaymentMetadata{TradeNo: event.TradeNo, PaymentProvider: provider, ExternalPaymentID: event.EventID, Metadata: string(metadataValue), CreateTime: time.Now().Unix(), UpdateTime: time.Now().Unix()}
	err = DB.Transaction(func(tx *gorm.DB) error {
		createErr := tx.Create(metadata).Error
		if createErr == nil {
			return nil
		} else if !isUniqueConstraintError(createErr) {
			return createErr
		}
		// If an active snapshot already claimed this chain event, the cancelled
		// snapshot is only an ambiguous late candidate. Do not make reconciliation
		// fail on the metadata uniqueness constraint and never replace the active
		// audit record with a non-crediting one.
		var usedEvent PaymentMetadata
		if lookupErr := tx.Where("payment_provider = ? AND external_payment_id = ?", provider, event.EventID).First(&usedEvent).Error; lookupErr == nil && usedEvent.TradeNo != event.TradeNo {
			return nil
		}
		// PaymentMetadata keeps one row per trade for compatibility with the
		// existing settlement callbacks. Preserve additional late chain events in
		// that row instead of silently dropping them on the trade_no unique key.
		var existing PaymentMetadata
		query := tx.Where("trade_no = ?", event.TradeNo)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if lookupErr := query.First(&existing).Error; lookupErr != nil || !strings.EqualFold(existing.PaymentProvider, provider) {
			return createErr
		}
		var existingMetadata map[string]interface{}
		if unmarshalErr := common.Unmarshal([]byte(existing.Metadata), &existingMetadata); unmarshalErr != nil {
			existingMetadata = map[string]interface{}{"legacy_metadata": existing.Metadata}
		}
		if existingMetadata["event_id"] == event.EventID {
			return nil
		}
		if events, ok := existingMetadata["late_events"].([]interface{}); ok {
			for _, value := range events {
				if item, ok := value.(map[string]interface{}); ok && item["event_id"] == event.EventID {
					return nil
				}
			}
			existingMetadata["late_events"] = append(events, eventMetadata)
		} else {
			existingMetadata = map[string]interface{}{"late_events": []interface{}{existingMetadata, eventMetadata}}
		}
		updatedMetadata, marshalErr := common.Marshal(existingMetadata)
		if marshalErr != nil {
			return marshalErr
		}
		return tx.Model(&PaymentMetadata{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{"metadata": string(updatedMetadata), "update_time": time.Now().Unix()}).Error
	})
	if err != nil {
		return err
	}
	return ErrDirectPaymentExpired
}

// CancelDirectCryptoPayment closes an unpaid invoice and its paired TopUp.
// It is intentionally user-scoped and idempotent for already-cancelled rows.
func CancelDirectCryptoPayment(tradeNo string, userID int) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		topUpQuery := tx.Where("trade_no = ? AND user_id = ?", strings.TrimSpace(tradeNo), userID)
		if tx.Dialector.Name() != "sqlite" {
			topUpQuery = topUpQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := topUpQuery.First(&topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		var payment DirectCryptoPayment
		paymentQuery := tx.Where("trade_no = ? AND user_id = ?", strings.TrimSpace(tradeNo), userID)
		if tx.Dialector.Name() != "sqlite" {
			paymentQuery = paymentQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := paymentQuery.First(&payment).Error; err != nil {
			return err
		}
		if payment.Status == DirectCryptoCancelled {
			return nil
		}
		if payment.Status != DirectCryptoPending || topUp.Status != common.TopUpStatusPending {
			return ErrDirectPaymentAlreadySettled
		}
		now := time.Now().Unix()
		if result := tx.Model(&TopUp{}).Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).Updates(map[string]interface{}{"status": common.TopUpStatusCancelled, "complete_time": now}); result.Error != nil {
			return result.Error
		} else if result.RowsAffected == 0 {
			return ErrDirectPaymentAlreadySettled
		}
		result := tx.Model(&DirectCryptoPayment{}).Where("id = ? AND status = ?", payment.Id, DirectCryptoPending).Updates(map[string]interface{}{"status": DirectCryptoCancelled, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrDirectPaymentAlreadySettled
		}
		return nil
	})
}

func sameDirectUSDTAddress(network, left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if strings.EqualFold(network, "TON") {
		lc, le := setting.CanonicalTONAddress(left)
		rc, re := setting.CanonicalTONAddress(right)
		return le == nil && re == nil && lc == rc
	}
	return left == right
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
			topUpQuery = topUpQuery.Clauses(clause.Locking{Strength: "UPDATE"})
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
			topUpQuery = topUpQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		topUpErr := topUpQuery.First(&topUp).Error
		if topUpErr != nil && !errors.Is(topUpErr, gorm.ErrRecordNotFound) {
			return topUpErr
		}

		paymentQuery := tx.Where("trade_no = ?", strings.TrimSpace(tradeNo))
		if tx.Dialector.Name() != "sqlite" {
			paymentQuery = paymentQuery.Clauses(clause.Locking{Strength: "UPDATE"})
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

func DirectUSDTNetworkEventID(network, txHash, eventIndex string) string {
	return strings.ToUpper(strings.TrimSpace(network)) + ":" + DirectUSDTEventID(txHash, eventIndex)
}

func DirectUSDTAmountString(units uint64) string {
	whole := units / 1_000_000
	frac := units % 1_000_000
	return strconv.FormatUint(whole, 10) + "." + fmt.Sprintf("%06d", frac)
}

// DirectUSDTAmountSuffixRange returns the suffix reservation space for a
// policy snapshot. Cents mode uses whole cent suffixes (0.01..0.99 USDT),
// while legacy mode retains the six-decimal range.
func DirectUSDTAmountSuffixRange(roundToCents bool) (minUnits, maxUnits, stepUnits int, err error) {
	if roundToCents {
		return 10_000, 990_000, 10_000, nil
	}
	// Older installations persisted only AmountPrecision. Honour that value
	// when the new tail-limit option is still at its default, preserving the
	// historical allocation space during migration.
	if setting.USDTTRC20AmountTailLimitUnits == setting.DefaultUSDTTRC20AmountTailLimitUnits && setting.USDTTRC20AmountPrecision != setting.DefaultUSDTTRC20AmountPrecision {
		if limit, e := setting.USDTTRC20AmountTailLimitForPrecision(setting.USDTTRC20AmountPrecision); e == nil {
			return 1, limit - 1, 1, nil
		}
	}
	return setting.USDTTRC20AmountSuffixRange()
}

func NormalizeDirectUSDTBaseUnits(units uint64, roundToCents bool) uint64 {
	if !roundToCents {
		return units
	}
	const cent = uint64(10_000)
	// Payment input is rounded half-up to cents before quota calculation. This
	// keeps the customer-visible amount and the immutable accounting snapshot
	// on the same side of the decimal boundary.
	if units > ^uint64(0)-cent/2 {
		// Avoid wrapping on malformed, impractically large input. Flooring still
		// produces a representable cent amount; the normal minimum/quote checks
		// reject values that cannot be used as a real invoice.
		return units / cent * cent
	}
	return (units + cent/2) / cent * cent
}

// directCryptoPaymentReservationIndex is used only by the additive migration.
// Keeping the index definition out of DirectCryptoPayment prevents AutoMigrate
// from creating the unique scoped index before legacy wallet keys are backfilled.
type directCryptoPaymentReservationIndex struct {
	WalletKey     string `gorm:"uniqueIndex:idx_direct_crypto_payment_wallet_amount,priority:1"`
	ExpectedUnits uint64 `gorm:"uniqueIndex:idx_direct_crypto_payment_wallet_amount,priority:2"`
}

func (directCryptoPaymentReservationIndex) TableName() string { return "direct_crypto_payments" }

// MigrateDirectCryptoPaymentIndexes removes the unique TxHash index from the
// early draft schema. A TRON transaction may contain multiple Transfer events;
// event identity is tx hash plus event index, while TxHash itself is not
// unique. The expected-amount uniqueness is intentionally preserved for
// rollback safety; the allocator chooses another suffix for another wallet.
func MigrateDirectCryptoPaymentIndexes(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	// Rows created before wallet pools had no wallet key. Backfill a stable
	// network/address key before creating the new composite reservation index;
	// this preserves their reservation identity while the global amount guard
	// remains available for rollback compatibility.
	var legacyRows []DirectCryptoPayment
	if err := db.Where("wallet_key = '' OR wallet_key IS NULL").Find(&legacyRows).Error; err != nil {
		return err
	}
	for _, row := range legacyRows {
		key := directUSDTWalletKey(row.Network, row.Address)
		if key == ":" {
			continue
		}
		if err := db.Model(&DirectCryptoPayment{}).Where("id = ? AND (wallet_key = '' OR wallet_key IS NULL)", row.Id).Update("wallet_key", key).Error; err != nil {
			return err
		}
	}
	createIndexIfMissing := func(model interface{}, name string) error {
		if db.Migrator().HasIndex(model, name) {
			return nil
		}
		if err := db.Migrator().CreateIndex(model, name); err != nil && !isIndexAlreadyExistsError(err) {
			return err
		}
		return nil
	}
	reservationIndex := &directCryptoPaymentReservationIndex{}
	if err := createIndexIfMissing(reservationIndex, "idx_direct_crypto_payment_wallet_amount"); err != nil {
		return err
	}
	indexes, indexErr := db.Migrator().GetIndexes(&DirectCryptoPayment{})
	if indexErr != nil {
		// Some older dialect adapters do not expose index metadata. In that case
		// recreate the two affected indexes explicitly below.
		indexes = nil
	}
	indexIsUnique := func(name string) bool {
		for _, index := range indexes {
			if index.Name() == name {
				unique, ok := index.Unique()
				return !ok || unique
			}
		}
		return false
	}
	for _, indexName := range []string{"idx_direct_crypto_payments_tx_hash", "idx_direct_crypto_payment_tx_hash"} {
		if !db.Migrator().HasIndex(&DirectCryptoPayment{}, indexName) {
			continue
		}
		// The early schema used a unique TxHash index. Drop it when metadata says
		// it is unique (or when the dialect cannot report metadata); a transaction
		// may legitimately contain multiple token transfer events.
		if indexName == "idx_direct_crypto_payment_tx_hash" && !indexIsUnique(indexName) && indexErr == nil {
			continue
		}
		if err := db.Migrator().DropIndex(&DirectCryptoPayment{}, indexName); err != nil && !isIndexAlreadyMissingError(err) {
			return err
		}
	}
	if err := ensureDirectCryptoExpectedUnitsUniqueIndex(db); err != nil {
		return err
	}
	return createIndexIfMissing(&DirectCryptoPayment{}, "idx_direct_crypto_payment_tx_hash")
}

func ensureDirectCryptoExpectedUnitsUniqueIndex(db *gorm.DB) error {
	const indexName = "idx_direct_crypto_payment_expected_units"
	if !db.Migrator().HasIndex(&DirectCryptoPayment{}, indexName) {
		return db.Migrator().CreateIndex(&DirectCryptoPayment{}, indexName)
	}
	indexes, err := db.Migrator().GetIndexes(&DirectCryptoPayment{})
	if err != nil {
		// Some SQLite adapters cannot report index metadata. The amount is
		// immutable, so first verify there are no duplicates, then recreate the
		// known index from the model tag as UNIQUE.
		if err := ensureNoDuplicateDirectCryptoAmounts(db); err != nil {
			return err
		}
		if err := db.Migrator().DropIndex(&DirectCryptoPayment{}, indexName); err != nil && !isIndexAlreadyMissingError(err) {
			return err
		}
		return db.Migrator().CreateIndex(&DirectCryptoPayment{}, indexName)
	}
	for _, index := range indexes {
		if index.Name() != indexName {
			continue
		}
		unique, known := index.Unique()
		if !known || unique {
			return nil
		}
		if err := ensureNoDuplicateDirectCryptoAmounts(db); err != nil {
			return err
		}
		if err := db.Migrator().DropIndex(&DirectCryptoPayment{}, indexName); err != nil && !isIndexAlreadyMissingError(err) {
			return err
		}
		return db.Migrator().CreateIndex(&DirectCryptoPayment{}, indexName)
	}
	return db.Migrator().CreateIndex(&DirectCryptoPayment{}, indexName)
}

func ensureNoDuplicateDirectCryptoAmounts(db *gorm.DB) error {
	var duplicate struct {
		ExpectedUnits uint64
		Count         int64
	}
	if result := db.Model(&DirectCryptoPayment{}).
		Select("expected_units, COUNT(*) AS count").
		Group("expected_units").
		Having("COUNT(*) > 1").
		Limit(1).
		Scan(&duplicate); result.Error != nil {
		return result.Error
	}
	if duplicate.Count > 0 {
		return fmt.Errorf("cannot restore unique direct USDT amount index: duplicate amount %d", duplicate.ExpectedUnits)
	}
	return nil
}

func isIndexAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "duplicate key name") ||
		strings.Contains(message, "duplicate index")
}

func isIndexAlreadyMissingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "does not exist") ||
		strings.Contains(message, "no such index") ||
		strings.Contains(message, "can't drop")
}
