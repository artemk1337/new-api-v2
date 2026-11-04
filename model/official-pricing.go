package model

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OfficialModelPrice is a developer's reference API price, never a billing
// fallback or a supplier's procurement cost. Records describe flat token rates.
type OfficialModelPrice struct {
	ModelName string   `json:"model_name" gorm:"primaryKey;size:255"`
	InputUSD  *float64 `json:"input_usd"`
	OutputUSD *float64 `json:"output_usd"`
	Source    string   `json:"source" gorm:"type:text"`
	UpdatedAt int64    `json:"updated_at" gorm:"autoUpdateTime:false"`
	PriceUnit string   `json:"price_unit" gorm:"size:32"`
}

// OfficialAPIPrice contains only public reference data, independent of suppliers.
type OfficialAPIPrice struct {
	InputUSD  float64 `json:"input_usd"`
	OutputUSD float64 `json:"output_usd"`
	Source    string  `json:"source"`
	UpdatedAt int64   `json:"updated_at"`
	PriceUnit string  `json:"price_unit"`
}

func ValidateOfficialModelPrice(price OfficialModelPrice) error {
	if price.ModelName == "" || len(price.ModelName) > 255 || strings.IndexFunc(price.ModelName, unicode.IsSpace) >= 0 {
		return errors.New("official model price requires an exact model name")
	}
	for _, rate := range []*float64{price.InputUSD, price.OutputUSD} {
		if rate == nil || math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate < 0 {
			return errors.New("official model price requires finite nonnegative input and output rates")
		}
	}
	if price.PriceUnit != "million_tokens" {
		return errors.New("official model price unit must be million_tokens")
	}
	if price.UpdatedAt <= 0 || price.UpdatedAt > time.Now().Unix() {
		return errors.New("official model price requires a valid quote timestamp")
	}
	return validateOfficialPriceSource(price.Source)
}

func validateOfficialPriceSource(source string) error {
	invalid := errors.New("official model price source must be a public HTTPS URL without credentials")
	if len(source) > 2048 {
		return invalid
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil {
		return invalid
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if port := parsed.Port(); port != "" && port != "443" {
		return invalid
	}
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return invalid
		}
		return nil
	}
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return invalid
	}
	// Browsers accept abbreviated and encoded IPv4 hosts. A numeric final
	// label is never a public DNS suffix and must not bypass the IP checks.
	suffix := host[strings.LastIndexByte(host, '.')+1:]
	if strings.IndexFunc(suffix, func(r rune) bool { return r >= 'a' && r <= 'z' }) < 0 {
		return invalid
	}
	for _, suffix := range []string{".localhost", ".local", ".localdomain", ".internal", ".test", ".invalid", ".example", ".home", ".arpa"} {
		if strings.HasSuffix(host, suffix) {
			return invalid
		}
	}
	return nil
}

func ListOfficialModelPrices() ([]OfficialModelPrice, error) {
	prices := make([]OfficialModelPrice, 0)
	err := DB.Order("model_name").Find(&prices).Error
	return prices, err
}

func GetOfficialAPIPrices(modelNames []string) (map[string]OfficialAPIPrice, error) {
	result := make(map[string]OfficialAPIPrice, len(modelNames))
	if len(modelNames) == 0 {
		return result, nil
	}
	var prices []OfficialModelPrice
	if err := DB.Where("model_name IN ?", modelNames).Find(&prices).Error; err != nil {
		return nil, err
	}
	for _, price := range prices {
		if ValidateOfficialModelPrice(price) != nil {
			continue
		}
		result[price.ModelName] = OfficialAPIPrice{
			InputUSD: *price.InputUSD, OutputUSD: *price.OutputUSD,
			Source: price.Source, UpdatedAt: price.UpdatedAt, PriceUnit: price.PriceUnit,
		}
	}
	return result, nil
}

func SaveOfficialModelPrices(prices []OfficialModelPrice) error {
	if len(prices) == 0 || len(prices) > 1000 {
		return errors.New("official model price batch must contain 1 to 1000 records")
	}
	seen := make(map[string]bool, len(prices))
	for index := range prices {
		prices[index].ModelName = strings.TrimSpace(prices[index].ModelName)
		prices[index].Source = strings.TrimSpace(prices[index].Source)
		if err := ValidateOfficialModelPrice(prices[index]); err != nil {
			return fmt.Errorf("record %d: %w", index+1, err)
		}
		if seen[prices[index].ModelName] {
			return errors.New("official model price batch contains duplicate models")
		}
		seen[prices[index].ModelName] = true
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "model_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"input_usd", "output_usd", "source", "updated_at", "price_unit"}),
		}).CreateInBatches(prices, 100).Error
	})
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

func DeleteOfficialModelPrice(modelName string) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || len(modelName) > 255 || strings.IndexFunc(modelName, unicode.IsSpace) >= 0 {
		return errors.New("official model price requires an exact model name")
	}
	if err := DB.Delete(&OfficialModelPrice{}, "model_name = ?", modelName).Error; err != nil {
		return err
	}
	InvalidatePricingCache()
	return nil
}
