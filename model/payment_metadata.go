package model

import "gorm.io/gorm"

type PaymentMetadata struct {
	Id                int    `json:"id"`
	TradeNo           string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentProvider   string `json:"payment_provider" gorm:"type:varchar(50);index;uniqueIndex:idx_payment_provider_external_id"`
	ExternalPaymentID string `json:"external_payment_id" gorm:"type:varchar(255);index;uniqueIndex:idx_payment_provider_external_id"`
	Metadata          string `json:"metadata" gorm:"type:text"`
	CreateTime        int64  `json:"create_time"`
	UpdateTime        int64  `json:"update_time"`
}

func (paymentMetadata *PaymentMetadata) Insert() error {
	return DB.Create(paymentMetadata).Error
}

func (paymentMetadata *PaymentMetadata) Update() error {
	return DB.Save(paymentMetadata).Error
}

func GetPaymentMetadataByTradeNo(tradeNo string) *PaymentMetadata {
	return getPaymentMetadataByTradeNo(DB, tradeNo)
}

func GetPaymentMetadataByTradeNoWithError(tradeNo string) (*PaymentMetadata, error) {
	return getPaymentMetadataByTradeNoWithError(DB, tradeNo)
}

// getPaymentMetadataByTradeNo reads metadata through the supplied database
// handle. Completion callbacks must pass their settlement transaction here:
// SQLite cannot acquire a second connection while the transaction holds the
// write lock.
func getPaymentMetadataByTradeNo(db *gorm.DB, tradeNo string) *PaymentMetadata {
	metadata, err := getPaymentMetadataByTradeNoWithError(db, tradeNo)
	if err != nil {
		return nil
	}
	return metadata
}

func getPaymentMetadataByTradeNoWithError(db *gorm.DB, tradeNo string) (*PaymentMetadata, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var paymentMetadata PaymentMetadata
	err := db.Where("trade_no = ?", tradeNo).First(&paymentMetadata).Error
	if err != nil {
		return nil, err
	}
	return &paymentMetadata, nil
}

func GetPaymentMetadataByExternalPaymentID(provider string, externalPaymentID string) *PaymentMetadata {
	metadata, err := GetPaymentMetadataByExternalPaymentIDWithError(provider, externalPaymentID)
	if err != nil {
		return nil
	}
	return metadata
}

func GetPaymentMetadataByExternalPaymentIDWithError(provider string, externalPaymentID string) (*PaymentMetadata, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var paymentMetadata PaymentMetadata
	err := DB.Where("payment_provider = ? AND external_payment_id = ?", provider, externalPaymentID).First(&paymentMetadata).Error
	if err != nil {
		return nil, err
	}
	return &paymentMetadata, nil
}
