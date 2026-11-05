package model

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnnouncementsTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "announcements.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}, &AnnouncementTelegramDelivery{}))
	previousDB, previousOptions := DB, common.OptionMap
	previousConsole := *console_setting.GetConsoleSetting()
	DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		DB, common.OptionMap = previousDB, previousOptions
		*console_setting.GetConsoleSetting() = previousConsole
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func announcementSettingsForTest(t *testing.T) AnnouncementSettings {
	t.Helper()
	settings, err := GetAnnouncementSettings()
	require.NoError(t, err)
	settings.Channel = "@example_news"
	settings.Announcements = []AnnouncementAdmin{{
		Announcement: Announcement{ID: "news-1", Content: "Short platform announcement", PublishDate: "2026-11-04T12:00:00Z"},
		TelegramMode: "bot", TelegramText: "Long Telegram post",
	}}
	return settings
}

func TestAnnouncementSaveIsAtomicAndRevisionChecksTelegramText(t *testing.T) {
	setupAnnouncementsTest(t)
	settings := announcementSettingsForTest(t)
	saved, err := SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	require.Equal(t, "pending", saved.Announcements[0].TelegramStatus)
	_, err = SaveAnnouncementSettings(settings)
	require.ErrorIs(t, err, ErrAnnouncementRevisionConflict)
	stale := saved
	saved.Announcements = append([]AnnouncementAdmin(nil), saved.Announcements...)
	saved.Announcements[0].TelegramText = "An edited Telegram post"
	changed, err := SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.NotEqual(t, stale.Revision, changed.Revision)
	_, err = SaveAnnouncementSettings(stale)
	require.ErrorIs(t, err, ErrAnnouncementRevisionConflict)
	changed.Announcements[0].Content = strings.Repeat("x", 5001)
	changed.Announcements[0].TelegramText = "This change must roll back too"
	_, err = SaveAnnouncementSettings(changed)
	require.Error(t, err)
	after, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Equal(t, "An edited Telegram post", after.Announcements[0].TelegramText)
	require.Equal(t, "Short platform announcement", after.Announcements[0].Content)
}

func TestAnnouncementDeliveryPreservesConcurrentEditAndOriginalChannel(t *testing.T) {
	setupAnnouncementsTest(t)
	saved, err := SaveAnnouncementSettings(announcementSettingsForTest(t))
	require.NoError(t, err)
	attempt, claimed, err := ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.True(t, claimed)
	_, claimed, err = ClaimAnnouncementTelegramDelivery("task-2")
	require.NoError(t, err)
	require.False(t, claimed)
	saved.Channel = "@new_channel"
	saved.Announcements[0].TelegramText = "Updated while sending"
	_, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.NoError(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "sent", ChatID: -100123, MessageID: 42}))
	next, claimed, err := ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "@example_news", next.Channel)
	require.Equal(t, int64(-100123), next.ChatID)
	require.Equal(t, 42, next.MessageID)
	require.Equal(t, "Updated while sending", next.AttemptText)
	require.Error(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "sent", MessageID: 99}))
	require.NoError(t, FinishAnnouncementTelegramDelivery(next, AnnouncementTelegramResult{Status: "sent", ChatID: -100123, MessageID: 42}))
	public, err := PublicAnnouncements()
	require.NoError(t, err)
	require.Equal(t, "https://t.me/example_news/42", public[0].TelegramURL)
	b, err := common.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(b), "Updated while sending")
	require.NotContains(t, string(b), "telegramStatus")
}

func TestAnnouncementInterruptedSendRequiresConfirmationAndCanUseExistingLink(t *testing.T) {
	setupAnnouncementsTest(t)
	_, err := SaveAnnouncementSettings(announcementSettingsForTest(t))
	require.NoError(t, err)
	_, claimed, err := ClaimAnnouncementTelegramDelivery("old-task")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, RecoverAnnouncementTelegramDeliveries("new-task"))
	_, claimed, err = ClaimAnnouncementTelegramDelivery("new-task")
	require.NoError(t, err)
	require.False(t, claimed)
	require.Error(t, RetryAnnouncementTelegramDelivery("news-1", false))
	saved, err := GetAnnouncementSettings()
	require.NoError(t, err)
	saved.Announcements[0].TelegramMode = "link"
	saved.Announcements[0].TelegramURL = "https://t.me/example_news/13"
	saved, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.Equal(t, "https://t.me/example_news/13", saved.Announcements[0].TelegramURL)
	saved.Announcements = nil
	_, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	var delivery AnnouncementTelegramDelivery
	require.NoError(t, DB.First(&delivery, "announcement_id = ?", "news-1").Error)
	require.True(t, delivery.Deleted)
	require.Zero(t, delivery.MessageID)
	require.Equal(t, "cancelled", delivery.Status)
}

func TestAnnouncementLegacyNumericIDsAndPublicOrdering(t *testing.T) {
	setupAnnouncementsTest(t)
	raw := `[{"id":1,"content":"Earlier","publishDate":"2026-11-04T12:30:00+03:00","type":"warning"},{"id":2,"content":"Later","publishDate":"2026-11-04T10:00:00Z"}]`
	require.NoError(t, UpdateOption(announcementsOption, raw))
	public, err := PublicAnnouncements()
	require.NoError(t, err)
	require.Equal(t, "2", public[0].ID)
	require.Equal(t, "1", public[1].ID)
	require.Equal(t, "warning", public[1].Type)
	private := `[{"id":"3","content":"Private","publishDate":"2026-11-04T10:00:00Z","telegramText":"secret"}]`
	require.Error(t, UpdateOption(announcementsOption, private))
	require.Error(t, UpdateOptionsBulk(map[string]string{announcementsOption: private}))
	settings, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Len(t, settings.Announcements, 2)
}

func TestAnnouncementLegacySaveCannotOverwriteBotURL(t *testing.T) {
	setupAnnouncementsTest(t)
	_, err := SaveAnnouncementSettings(announcementSettingsForTest(t))
	require.NoError(t, err)
	attempt, _, err := ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.NoError(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "sent", MessageID: 7}))
	raw := `[{"id":"news-1","content":"Legacy content edit","publishDate":"2026-11-04T12:00:00Z","telegramUrl":"https://t.me/another_channel/3"}]`
	require.NoError(t, UpdateOptionsBulk(map[string]string{announcementsOption: raw}))
	settings, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Equal(t, "bot", settings.Announcements[0].TelegramMode)
	require.Equal(t, "https://t.me/example_news/7", settings.Announcements[0].TelegramURL)
	require.Equal(t, "Legacy content edit", settings.Announcements[0].Content)
}

func TestAnnouncementUnknownOutcomeSurvivesModeChanges(t *testing.T) {
	setupAnnouncementsTest(t)
	_, err := SaveAnnouncementSettings(announcementSettingsForTest(t))
	require.NoError(t, err)
	_, _, err = ClaimAnnouncementTelegramDelivery("old-task")
	require.NoError(t, err)
	saved, err := GetAnnouncementSettings()
	require.NoError(t, err)
	saved.Announcements[0].TelegramMode = "platform"
	saved, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.NoError(t, RecoverAnnouncementTelegramDeliveries("new-task"))
	saved, err = GetAnnouncementSettings()
	require.NoError(t, err)
	require.Equal(t, "unknown", saved.Announcements[0].TelegramStatus)
	saved, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.Equal(t, "unknown", saved.Announcements[0].TelegramStatus)
	saved.Announcements[0].TelegramMode = "bot"
	saved, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.Equal(t, "unknown", saved.Announcements[0].TelegramStatus)
	_, claimed, err := ClaimAnnouncementTelegramDelivery("new-task")
	require.NoError(t, err)
	require.False(t, claimed)
	require.Error(t, RetryAnnouncementTelegramDelivery("news-1", false))
	require.NoError(t, RetryAnnouncementTelegramDelivery("news-1", true))
	_, claimed, err = ClaimAnnouncementTelegramDelivery("new-task")
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestAnnouncementLegacyKeyMigratesOnlyWhenCurrentOptionIsAbsent(t *testing.T) {
	setupAnnouncementsTest(t)
	raw := `[{"id":7,"content":"Legacy announcement","publishDate":"2026-11-04T12:00:00Z"}]`
	require.NoError(t, DB.Create(&Option{Key: "Announcements", Value: raw}).Error)
	settings, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Len(t, settings.Announcements, 1)
	require.Equal(t, "7", settings.Announcements[0].ID)
	settings.Announcements = nil
	_, err = SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	settings, err = GetAnnouncementSettings()
	require.NoError(t, err)
	require.Empty(t, settings.Announcements)
}

func TestAnnouncementFailedCreateCanUseCorrectedChannelWithoutAutomaticRetry(t *testing.T) {
	setupAnnouncementsTest(t)
	_, err := SaveAnnouncementSettings(announcementSettingsForTest(t))
	require.NoError(t, err)
	attempt, claimed, err := ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, FinishAnnouncementTelegramDelivery(attempt, AnnouncementTelegramResult{Status: "failed", Error: "Channel not found"}))
	saved, err := GetAnnouncementSettings()
	require.NoError(t, err)
	saved.Channel = "@correct_channel"
	saved, err = SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.Equal(t, "failed", saved.Announcements[0].TelegramStatus)
	_, claimed, err = ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, RetryAnnouncementTelegramDelivery("news-1", false))
	attempt, claimed, err = ClaimAnnouncementTelegramDelivery("task-1")
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "@correct_channel", attempt.Channel)
}

func TestAnnouncementTitlePersistsSeparatelyFromContent(t *testing.T) {
	setupAnnouncementsTest(t)
	settings := announcementSettingsForTest(t)
	settings.Announcements[0].Title = "Новые модели <API> **без Markdown**"
	saved, err := SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	require.Equal(t, settings.Announcements[0].Title, saved.Announcements[0].Title)
	require.Equal(t, "Short platform announcement", saved.Announcements[0].Content)
	public, err := PublicAnnouncements()
	require.NoError(t, err)
	require.Equal(t, settings.Announcements[0].Title, public[0].Title)
	var delivery AnnouncementTelegramDelivery
	require.NoError(t, DB.First(&delivery, "announcement_id = ?", "news-1").Error)
	require.Equal(t, "Long Telegram post", delivery.DesiredText)

	previousRevision := saved.Revision
	saved.Announcements[0].Title = "Updated title"
	changed, err := SaveAnnouncementSettings(saved)
	require.NoError(t, err)
	require.NotEqual(t, previousRevision, changed.Revision)
	reloaded, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Equal(t, "Updated title", reloaded.Announcements[0].Title)
	require.Equal(t, "Short platform announcement", reloaded.Announcements[0].Content)
}

func TestAnnouncementTitleLimitCountsUnicodeCharacters(t *testing.T) {
	setupAnnouncementsTest(t)
	settings := announcementSettingsForTest(t)
	settings.Announcements[0].Title = strings.Repeat("я", 199) + "🚀"
	saved, err := SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	require.Equal(t, settings.Announcements[0].Title, saved.Announcements[0].Title)
	saved.Announcements[0].Title += "!"
	_, err = SaveAnnouncementSettings(saved)
	require.EqualError(t, err, "Announcement title must not exceed 200 characters")
	after, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Equal(t, settings.Announcements[0].Title, after.Announcements[0].Title)
}

func TestAnnouncementWithoutTitleRemainsCompatible(t *testing.T) {
	setupAnnouncementsTest(t)
	raw := `[{"id":"legacy-news","content":"**Legacy heading**\n\nLegacy content","publishDate":"2026-11-04T12:00:00Z"}]`
	require.NoError(t, UpdateOption(announcementsOption, raw))
	settings, err := GetAnnouncementSettings()
	require.NoError(t, err)
	require.Empty(t, settings.Announcements[0].Title)
	saved, err := SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	require.Equal(t, "**Legacy heading**\n\nLegacy content", saved.Announcements[0].Content)
	public, err := PublicAnnouncements()
	require.NoError(t, err)
	require.Equal(t, saved.Announcements[0].Announcement, public[0])
}
