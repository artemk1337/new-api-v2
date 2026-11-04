package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/stretchr/testify/require"
)

func TestAnnouncementPersistenceDialects(t *testing.T) {
	for _, envName := range []string{"ANNOUNCEMENT_TEST_POSTGRES_DSN", "ANNOUNCEMENT_TEST_MYSQL_DSN"} {
		t.Run(envName, func(t *testing.T) {
			if os.Getenv(envName) == "" {
				t.Skip("isolated dialect database is not configured")
			}
			db, kind, err := chooseDB(envName, false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.Empty(t, tables, "announcement dialect tests require an empty isolated database")
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&AnnouncementTelegramDelivery{}, &Option{})) })
			previousDB, previousOptions := DB, common.OptionMap
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			previousConsole := *console_setting.GetConsoleSetting()
			DB = db
			common.OptionMap = map[string]string{}
			common.SetMainDatabaseType(kind)
			t.Cleanup(func() {
				DB = previousDB
				common.OptionMap = previousOptions
				*console_setting.GetConsoleSetting() = previousConsole
				common.SetDatabaseTypes(previousMain, previousLog)
			})
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&Option{}, &AnnouncementTelegramDelivery{}))
			}
			settings := announcementSettingsForTest(t)
			settings.Announcements[0].TelegramText = strings.Repeat("😀", 2048)
			saved, err := SaveAnnouncementSettings(settings)
			require.NoError(t, err)
			_, err = SaveAnnouncementSettings(settings)
			require.ErrorIs(t, err, ErrAnnouncementRevisionConflict)
			attempt, claimed, err := ClaimAnnouncementTelegramDelivery("dialect-task")
			require.NoError(t, err)
			require.True(t, claimed)
			require.Equal(t, settings.Announcements[0].TelegramText, attempt.AttemptText)
			require.NoError(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "sent", ChatID: -100123, MessageID: 43}))
			saved, err = GetAnnouncementSettings()
			require.NoError(t, err)
			require.Equal(t, "https://t.me/example_news/43", saved.Announcements[0].TelegramURL)
			saved.Announcements[0].TelegramText = "Edited text"
			saved, err = SaveAnnouncementSettings(saved)
			require.NoError(t, err)
			attempt, claimed, err = ClaimAnnouncementTelegramDelivery("old-task")
			require.NoError(t, err)
			require.True(t, claimed)
			require.NoError(t, RecoverAnnouncementTelegramDeliveries("new-task"))
			attempt, claimed, err = ClaimAnnouncementTelegramDelivery("new-task")
			require.NoError(t, err)
			require.True(t, claimed)
			require.Equal(t, 43, attempt.MessageID)
			require.Equal(t, "Edited text", attempt.AttemptText)
			require.NoError(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "sent", ChatID: -100123, MessageID: 43}))
			current, err := GetAnnouncementSettings()
			require.NoError(t, err)
			current.Announcements = nil
			_, err = SaveAnnouncementSettings(current)
			require.NoError(t, err)
			var delivery AnnouncementTelegramDelivery
			require.NoError(t, db.First(&delivery, "announcement_id = ?", "news-1").Error)
			require.True(t, delivery.Deleted)
			require.Equal(t, 43, delivery.MessageID)
		})
	}
}
