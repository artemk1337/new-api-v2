package model

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var logSupplierColumnCache sync.Map

// SetLogProxySupplier freezes procurement identity inside private log metadata.
// The sidecar survives outbox JSON serialization and pre-migration writers.
func SetLogProxySupplier(other map[string]interface{}, supplierID int) {
	admin, ok := other["admin_info"].(map[string]interface{})
	if !ok {
		admin = make(map[string]interface{})
		other["admin_info"] = admin
	}
	admin["proxy_supplier_id"] = supplierID
	if supplierID > 0 {
		other["supplier_pricing"] = true
	}
}

func captureLogProxySupplier(log *Log, fallbackID int) bool {
	var snapshot struct {
		AdminInfo struct {
			ProxySupplierID *int `json:"proxy_supplier_id"`
		} `json:"admin_info"`
	}
	known := common.UnmarshalJsonStr(log.Other, &snapshot) == nil && snapshot.AdminInfo.ProxySupplierID != nil
	if known {
		fallbackID = *snapshot.AdminInfo.ProxySupplierID
	}
	if !known && fallbackID <= 0 {
		log.ProxySupplierID = 0
		return false
	}
	if fallbackID < 0 {
		fallbackID = 0
	}
	log.ProxySupplierID = fallbackID
	other, err := common.StrToMap(log.Other)
	if err != nil && fallbackID == 0 {
		return true
	}
	if other == nil {
		other = make(map[string]interface{})
	}
	SetLogProxySupplier(other, fallbackID)
	log.Other = common.MapToJsonStr(other)
	return true
}

// Legacy callers can lack attribution. Preserve their privacy classification
// without treating the current channel binding as historical identity.
func classifyLegacySupplierLog(log *Log) {
	if log.ChannelId <= 0 {
		return
	}
	other, _ := common.StrToMap(log.Other)
	if supplier, _ := other["supplier_pricing"].(bool); supplier {
		return
	}
	var channel Channel
	if err := DB.Select("supplier_id").First(&channel, "id = ?", log.ChannelId).Error; err != nil || channel.SupplierID == nil {
		return
	}
	if other == nil {
		other = make(map[string]interface{})
	}
	other["supplier_pricing"] = true
	log.Other = common.MapToJsonStr(other)
}

func logSupplierWriteDB(tx *gorm.DB) *gorm.DB {
	if !logSupplierColumnReady(tx) {
		return tx.Omit("proxy_supplier_id")
	}
	return tx
}

func logSupplierColumnReady(db *gorm.DB) bool {
	pool, err := db.DB()
	if err != nil {
		return db.Migrator().HasColumn(&Log{}, "proxy_supplier_id")
	}
	if _, ready := logSupplierColumnCache.Load(pool); ready {
		return true
	}
	if !db.Migrator().HasColumn(&Log{}, "proxy_supplier_id") {
		return false
	}
	logSupplierColumnCache.Store(pool, true)
	return true
}

func invalidateLogSupplierColumnCache(db *gorm.DB) {
	if pool, err := db.DB(); err == nil {
		logSupplierColumnCache.Delete(pool)
	}
}
