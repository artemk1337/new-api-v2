package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRegistrationIPStorageAndJSON(t *testing.T) {
	truncateTables(t)
	for _, tc := range []struct {
		input, canonical string
		size             int
	}{
		{"1.1.1.1", "1.1.1.1", 4},
		{"2001:0db8:0000::1", "2001:db8::1", 16},
		{"::ffff:1.1.1.1", "1.1.1.1", 4},
		{"", "", 0},
		{"invalid", "", 0},
		{"fe80::1%en0", "", 0},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ip := ParseRegistrationIP(tc.input)
			require.Len(t, ip, tc.size)
			user := User{Username: "ip-" + tc.input, AffCode: "ip-" + tc.input, RegistrationIP: ip}
			require.NoError(t, DB.Create(&user).Error)
			var loaded User
			require.NoError(t, DB.First(&loaded, user.Id).Error)
			require.Equal(t, tc.canonical, loaded.RegistrationIP.String())
			var raw []byte
			require.NoError(t, DB.Raw("SELECT registration_ip FROM users WHERE id = ?", user.Id).Row().Scan(&raw))
			require.Len(t, raw, tc.size)
			if tc.size == 0 {
				require.Nil(t, raw)
			}
			encoded, err := common.Marshal(loaded)
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, common.Unmarshal(encoded, &response))
			require.Equal(t, tc.canonical, response["registration_ip"])
			loaded.RegistrationIP = ParseRegistrationIP("8.8.8.8")
			loaded.DisplayName = "Updated name"
			require.NoError(t, DB.Save(&loaded).Error)
			loaded = User{}
			require.NoError(t, DB.First(&loaded, user.Id).Error)
			require.Equal(t, tc.canonical, loaded.RegistrationIP.String())
			require.Equal(t, "Updated name", loaded.DisplayName)
		})
	}
}

func TestSearchUsersRegistrationIPPreservesFilters(t *testing.T) {
	truncateTables(t)
	for _, user := range []User{
		{Username: "ipv4", AffCode: "ipv4", RegistrationIP: ParseRegistrationIP("1.1.1.1"), Group: "default", Role: 1, Status: 1},
		{Username: "ipv6", AffCode: "ipv6", RegistrationIP: ParseRegistrationIP("2001:db8::1"), Group: "other", Role: 2, Status: 2},
		{Username: "text-1.1.1.1", AffCode: "text"},
	} {
		require.NoError(t, DB.Create(&user).Error)
	}
	users, total, err := SearchUsers("::ffff:1.1.1.1", "default", nil, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "ipv4", users[0].Username)
	role, status := 2, 2
	users, total, err = SearchUsers("2001:0db8:0::1", "other", &role, &status, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "ipv6", users[0].Username)
	_, total, err = SearchUsers("2001:db8::1", "default", nil, nil, 0, 10)
	require.NoError(t, err)
	require.Zero(t, total)
	_, total, err = SearchUsers("1.1.1.1", "", nil, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	_, total, err = SearchUsers("2001:db8::", "", nil, nil, 0, 10)
	require.NoError(t, err)
	require.Zero(t, total)
}

func TestRegistrationIPDialectTypes(t *testing.T) {
	for _, tc := range []struct {
		dialector gorm.Dialector
		expected  string
	}{
		{sqlite.Open(":memory:"), "blob"},
		{mysql.New(mysql.Config{SkipInitializeWithVersion: true}), "varbinary(16)"},
		{postgres.New(postgres.Config{}), "bytea"},
	} {
		db := &gorm.DB{Config: &gorm.Config{Dialector: tc.dialector}}
		require.Equal(t, tc.expected, (RegistrationIP(nil)).GormDBDataType(db, nil))
	}
}

func TestRegistrationIPMigrationPreservesLegacyUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Migrator().DropColumn(&User{}, "registration_ip"))
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password, aff_code) VALUES (1, 'legacy', 'password', 'legacy')").Error)
	require.NoError(t, db.AutoMigrate(&User{}))
	var user User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, "legacy", user.Username)
	require.Nil(t, user.RegistrationIP)
	var raw []byte
	require.NoError(t, db.Raw("SELECT registration_ip FROM users WHERE id = 1").Row().Scan(&raw))
	require.Nil(t, raw)
	// Old application inserts remain valid after the additive migration.
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password, aff_code) VALUES (2, 'old-app', 'password', 'old-app')").Error)
}
