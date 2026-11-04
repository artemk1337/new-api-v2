package model

import (
	"errors"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

type LogFilter struct {
	Type              int
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	Username          string
	TokenName         string
	Channel           int
	Group             string
	RequestID         string
	UpstreamRequestID string
	ProxySupplierID   *int
	UserID            int
}

func queryLogs(filter LogFilter) (*gorm.DB, error) {
	return queryLogsDB(LOG_DB, filter)
}

func queryLogsDB(db *gorm.DB, filter LogFilter) (*gorm.DB, error) {
	tx := db.Model(&Log{})
	if filter.UserID > 0 {
		tx = tx.Where("logs.user_id = ?", filter.UserID)
	}
	if filter.Type != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", filter.Type)
	}
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", filter.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", filter.Username); err != nil {
		return nil, err
	}
	for _, condition := range []struct{ column, value string }{
		{"logs.token_name", filter.TokenName},
		{"logs.request_id", filter.RequestID}, {"logs.upstream_request_id", filter.UpstreamRequestID},
	} {
		if condition.value != "" {
			tx = tx.Where(condition.column+" = ?", condition.value)
		}
	}
	if filter.StartTimestamp != 0 {
		// Timestamp bounds also constrain throughput and suggestion queries.
		tx = tx.Where("logs.created_at >= ?", filter.StartTimestamp)
	}
	if filter.Group != "" {
		groups := []string{filter.Group}
		if canonical := ratio_setting.PricingGroupKey(filter.Group); canonical != filter.Group {
			groups = append(groups, canonical)
		}
		tx = tx.Where("logs."+logGroupCol+" IN ?", groups)
	}
	if filter.EndTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", filter.EndTimestamp)
	}
	if filter.Channel != 0 {
		tx = tx.Where("logs.channel_id = ?", filter.Channel)
	}
	if filter.ProxySupplierID != nil {
		if !logSupplierColumnReady(db) {
			if *filter.ProxySupplierID == 0 {
				return tx, nil
			}
			return tx.Where("1 = 0"), nil
		}
		if *filter.ProxySupplierID < 0 {
			return nil, errors.New("invalid proxy supplier ID")
		}
		tx = tx.Where("logs.proxy_supplier_id = ?", *filter.ProxySupplierID)
	}
	return tx, nil
}
