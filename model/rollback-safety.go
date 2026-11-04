package model

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func rollbackPricingBlocked(detail string) error {
	return fmt.Errorf("legacy pricing rollback blocked: %s; prepare the database with supplier-aware v1.1.218, restore reviewed legacy group coefficients, detach native bindings, and archive native tasks/outbox separately while preserving their data; no data was changed", detail)
}

// CheckLegacyPricingRollbackSafety refuses native contracts that this release
// cannot route, settle or expose safely. It never rewrites persisted data.
func CheckLegacyPricingRollbackSafety() error {
	return checkLegacyPricingRollbackSafety(DB)
}

func checkLegacyPricingRollbackSafety(db *gorm.DB) error {
	tableNames, err := db.Migrator().GetTables()
	if err != nil {
		return errors.New("cannot verify legacy pricing rollback safety: database schema is unavailable")
	}
	tables := make(map[string]bool, len(tableNames))
	for _, name := range tableNames {
		tables[name] = true
	}
	if tables["pricing_group_supplier_bindings"] {
		var count int64
		if err := db.Table("pricing_group_supplier_bindings").Count(&count).Error; err != nil {
			return errors.New("cannot verify legacy pricing rollback safety: group bindings are unreadable")
		}
		if count > 0 {
			return rollbackPricingBlocked("native group bindings remain")
		}
	}
	if tables["channels"] {
		columns, err := db.Migrator().ColumnTypes("channels")
		if err != nil {
			return errors.New("cannot verify legacy pricing rollback safety: channel schema is unreadable")
		}
		var conditions []string
		for _, column := range columns {
			if column.Name() == "supplier_id" || column.Name() == "supplier_profile_id" {
				conditions = append(conditions, column.Name()+" IS NOT NULL AND "+column.Name()+" <> 0")
			}
		}
		if len(conditions) > 0 {
			query := db.Table("channels").Where(conditions[0])
			for _, condition := range conditions[1:] {
				query = query.Or(condition)
			}
			var count int64
			if err := query.Count(&count).Error; err != nil {
				return errors.New("cannot verify legacy pricing rollback safety: channel bindings are unreadable")
			}
			if count > 0 {
				return rollbackPricingBlocked("native channel bindings remain, including disabled channels")
			}
		}
	}
	if tables["tasks"] {
		if err := checkRollbackTaskContracts(db); err != nil {
			return err
		}
	}
	if tables["billing_outboxes"] {
		return checkRollbackOutboxContracts(db)
	}
	return nil
}

func checkRollbackTaskContracts(db *gorm.DB) error {
	rows, err := db.Table("tasks").Select("id, private_data").Rows()
	if err != nil {
		return errors.New("cannot verify legacy pricing rollback safety: task snapshots are unreadable")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var data sql.NullString
		if err := rows.Scan(&id, &data); err != nil {
			return errors.New("cannot verify legacy pricing rollback safety: task snapshot could not be read")
		}
		if !data.Valid || data.String == "" {
			continue
		}
		var snapshot struct {
			BillingContext struct {
				SupplierID    int `json:"supplier_id"`
				ProfileID     int `json:"profile_id"`
				PriceSnapshot struct {
					SupplierTariff json.RawMessage
				} `json:"price_snapshot"`
			} `json:"billing_context"`
		}
		if common.UnmarshalJsonStr(data.String, &snapshot) != nil {
			return rollbackPricingBlocked(fmt.Sprintf("task %d has an unverifiable snapshot", id))
		}
		context := snapshot.BillingContext
		tariff := bytes.TrimSpace(context.PriceSnapshot.SupplierTariff)
		if context.SupplierID != 0 || context.ProfileID != 0 || len(tariff) > 0 && !bytes.Equal(tariff, []byte("null")) {
			return rollbackPricingBlocked(fmt.Sprintf("task %d retains a native pricing contract, including historical tasks", id))
		}
	}
	if rows.Err() != nil {
		return errors.New("cannot verify legacy pricing rollback safety: task scan failed")
	}
	return nil
}

func checkRollbackOutboxContracts(db *gorm.DB) error {
	rows, err := db.Table("billing_outboxes").Select("id, kind, payload").Rows()
	if err != nil {
		return errors.New("cannot verify legacy pricing rollback safety: billing recovery queue is unreadable")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var kind, payload string
		if err := rows.Scan(&id, &kind, &payload); err != nil {
			return errors.New("cannot verify legacy pricing rollback safety: billing recovery record could not be read")
		}
		if kind != BillingOutboxKindLog {
			continue
		}
		var record billingLogOutboxPayload
		if common.UnmarshalJsonStr(payload, &record) != nil {
			return rollbackPricingBlocked(fmt.Sprintf("billing outbox %d has an unverifiable log snapshot", id))
		}
		if record.Log.Other == "" {
			continue
		}
		other, err := common.StrToMap(record.Log.Other)
		if err != nil {
			return rollbackPricingBlocked(fmt.Sprintf("billing outbox %d has unverifiable log metadata", id))
		}
		if record.Log.LegacyProxySupplierID != 0 || historicalSupplierMetadata(other) {
			return rollbackPricingBlocked(fmt.Sprintf("billing outbox %d retains native supplier metadata", id))
		}
	}
	if rows.Err() != nil {
		return errors.New("cannot verify legacy pricing rollback safety: billing recovery scan failed")
	}
	return nil
}
