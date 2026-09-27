package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DirectCryptoReconciliationOrphan         = "orphan"
	DirectCryptoReconciliationAmountMismatch = "amount_mismatch"
)

// DirectCryptoReconciliation is a durable, non-crediting record for a
// verified chain transfer that cannot be matched to an exact invoice. The
// event key is unique per network, so watcher retries and multiple instances
// cannot create duplicate manual-review items.
type DirectCryptoReconciliation struct {
	Id             uint   `gorm:"primaryKey" json:"id"`
	Network        string `gorm:"type:varchar(16);not null;uniqueIndex:idx_direct_crypto_reconciliation_event" json:"network"`
	EventID        string `gorm:"type:varchar(255);not null;uniqueIndex:idx_direct_crypto_reconciliation_event" json:"event_id"`
	TradeNo        string `gorm:"type:varchar(255);index" json:"trade_no,omitempty"`
	TxHash         string `gorm:"type:varchar(128);not null" json:"tx_hash"`
	EventIndex     string `gorm:"type:varchar(64);not null" json:"event_index"`
	Contract       string `gorm:"type:varchar(128);not null" json:"contract"`
	Source         string `gorm:"type:varchar(128)" json:"source,omitempty"`
	Destination    string `gorm:"type:varchar(128);not null" json:"destination"`
	AmountUnits    uint64 `gorm:"not null" json:"amount_units"`
	ExpectedUnits  uint64 `gorm:"not null" json:"expected_units"`
	Reason         string `gorm:"type:varchar(32);not null;index" json:"reason"`
	BlockTimestamp int64  `gorm:"index;not null" json:"block_timestamp"`
	CreatedAt      int64  `gorm:"index;not null" json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// TableName keeps the audit table name stable across GORM naming changes.
func (DirectCryptoReconciliation) TableName() string { return "direct_crypto_reconciliations" }

// RecordDirectUSDTReconciliationEvent stores a verified, non-crediting chain
// event. Empty TradeNo is valid for an orphan transfer; when one candidate
// invoice exists, callers may include it as context for support review.
func RecordDirectUSDTReconciliationEvent(event DirectUSDTTransferEvent, reason, tradeNo string, expectedUnits uint64) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(event.Network) == "" {
		event.Network = "TRON"
	}
	event = event.normalized()
	if !isDirectPaymentChainEventValid(event) || strings.TrimSpace(event.To) == "" || !validDirectUSDTNetwork(event.Network, event.Contract) {
		return ErrDirectPaymentInvalid
	}
	if reason != DirectCryptoReconciliationOrphan && reason != DirectCryptoReconciliationAmountMismatch {
		return errors.New("invalid direct USDT reconciliation reason")
	}
	blockUnix := event.BlockTimestamp
	if strings.EqualFold(event.Network, "TRON") {
		blockUnix /= 1000
	}
	if blockUnix <= 0 || blockUnix > time.Now().Unix() {
		return ErrDirectPaymentInvalid
	}
	now := time.Now().Unix()
	record := &DirectCryptoReconciliation{
		Network: event.Network, EventID: event.EventID,
		TradeNo: strings.TrimSpace(tradeNo), TxHash: event.TxHash,
		EventIndex: event.EventIndex, Contract: event.Contract, Source: event.Source,
		Destination: event.To, AmountUnits: event.AmountUnits, ExpectedUnits: expectedUnits,
		Reason: reason, BlockTimestamp: blockUnix, CreatedAt: now, UpdatedAt: now,
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockDirectUSDTEventDecision(tx); err != nil {
			return err
		}
		var settled int64
		if err := tx.Model(&DirectCryptoPayment{}).
			Where("event_id = ? AND status = ?", event.EventID, DirectCryptoPaid).
			Count(&settled).Error; err != nil {
			return err
		}
		if settled > 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "network"}, {Name: "event_id"}},
			DoNothing: true,
		}).Create(record).Error
	})
}

// IsDirectUSDTEventSettled prevents a later reconciliation pass from
// labelling a transfer that already credited an invoice as orphaned.
func IsDirectUSDTEventSettled(eventID string) (bool, error) {
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	var count int64
	err := DB.Model(&DirectCryptoPayment{}).
		Where("event_id = ? AND status = ?", strings.TrimSpace(eventID), DirectCryptoPaid).
		Count(&count).Error
	return count > 0, err
}
