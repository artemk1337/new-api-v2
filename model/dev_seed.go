package model

import (
	"os"

	"github.com/QuantumNous/new-api/common"
)

// devSeedBuildMarker is set only by Dockerfile.dev at build time. Keeping the
// marker in the binary prevents a production image from enabling this seed by
// accidentally inheriting NEW_API_DEV_SEED.
var devSeedBuildMarker = "false"

// SeedDevData creates deliberately local-only fixtures for docker-compose.dev.
// It is opt-in and never runs unless NEW_API_DEV_SEED=true is explicitly set.
func SeedDevData() error {
	if devSeedBuildMarker != "true" || os.Getenv("NEW_API_DEV_SEED") != "true" {
		return nil
	}
	if err := SeedDefaultPlatformCurrencies(); err != nil {
		return err
	}

	var admin User
	result := DB.Where("username = ?", "admin").Limit(1).Find(&admin)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		hashedPassword, hashErr := common.Password2Hash("admin")
		if hashErr != nil {
			return hashErr
		}
		admin = User{
			Username:    "admin",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Dev Superadmin",
			Quota:       100000000,
		}
		if err := DB.Create(&admin).Error; err != nil {
			return err
		}
	}

	// Keep the methods visible in the wallet, but point the legacy EPay
	// callback at an intentionally local, non-provider endpoint. No real
	// payment credentials or external checkout are enabled by this fixture.
	if err := ensureDevOption("PayAddress", "http://127.0.0.1:3999"); err != nil {
		return err
	}
	if err := ensureDevOption("EpayId", "dev"); err != nil {
		return err
	}
	if err := ensureDevOption("EpayKey", "dev"); err != nil {
		return err
	}
	devPayMethods := []map[string]string{
		{"name": "支付宝", "icon": "SiAlipay", "type": "alipay", "currency": "USD"},
		{"name": "微信", "icon": "SiWechat", "type": "wxpay", "currency": "USD"},
	}
	encodedPayMethods, err := common.Marshal(devPayMethods)
	if err != nil {
		return err
	}
	if err := ensureDevOption("PayMethods", string(encodedPayMethods)); err != nil {
		return err
	}
	return nil
}

func ensureDevOption(key, value string) error {
	var option Option
	result := DB.Where("key = ?", key).Limit(1).Find(&option)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	return UpdateOption(key, value)
}
