package system_setting

import (
	_ "embed"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

//go:embed default_user_agreement.md
var defaultUserAgreement string

type LegalSettings struct {
	UserAgreement string `json:"user_agreement"`
	PrivacyPolicy string `json:"privacy_policy"`
}

var defaultLegalSettings = LegalSettings{
	UserAgreement: defaultUserAgreement,
	PrivacyPolicy: "",
}

func init() {
	config.GlobalConfig.Register("legal", &defaultLegalSettings)
}

func GetLegalSettings() LegalSettings {
	settings := defaultLegalSettings
	if strings.TrimSpace(settings.UserAgreement) == "" {
		settings.UserAgreement = defaultUserAgreement
	}
	return settings
}
