package model

import (
	"database/sql/driver"
	"fmt"
	"net/netip"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// RegistrationIP stores only the address bytes, without padding IPv4 to IPv6.
type RegistrationIP []byte

func ParseRegistrationIP(value string) RegistrationIP {
	address, err := netip.ParseAddr(value)
	if err != nil || address.Zone() != "" {
		return nil
	}
	return RegistrationIP(address.Unmap().AsSlice())
}

func (ip RegistrationIP) String() string {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return ""
	}
	return address.Unmap().String()
}

func (ip RegistrationIP) Value() (driver.Value, error) {
	if len(ip) == 0 {
		return nil, nil
	}
	if len(ip) != 4 && len(ip) != 16 {
		return nil, fmt.Errorf("invalid registration IP length: %d", len(ip))
	}
	return []byte(ip), nil
}

func (ip *RegistrationIP) Scan(value any) error {
	if value == nil {
		*ip = nil
		return nil
	}
	var raw []byte
	switch value := value.(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return fmt.Errorf("invalid registration IP database type: %T", value)
	}
	if len(raw) != 4 && len(raw) != 16 {
		return fmt.Errorf("invalid registration IP length: %d", len(raw))
	}
	*ip = append(RegistrationIP(nil), raw...)
	return nil
}

func (ip RegistrationIP) MarshalJSON() ([]byte, error) { return common.Marshal(ip.String()) }

func (ip *RegistrationIP) UnmarshalJSON(data []byte) error {
	var value string
	if err := common.Unmarshal(data, &value); err != nil {
		return err
	}
	*ip = ParseRegistrationIP(value)
	return nil
}

func (RegistrationIP) GormDataType() string { return "bytes" }

func (RegistrationIP) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "varbinary(16)"
	case "postgres":
		return "bytea"
	default:
		return "blob"
	}
}
