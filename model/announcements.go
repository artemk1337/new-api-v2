package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const announcementsOption = "console_setting.announcements"
const telegramChannelOption = "console_setting.telegram_channel"

var ErrAnnouncementRevisionConflict = errors.New("announcements changed; reload before saving")
var announcementIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Announcement struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	PublishDate string `json:"publishDate"`
	Type        string `json:"type,omitempty"`
	Extra       string `json:"extra,omitempty"`
	TelegramURL string `json:"telegramUrl,omitempty"`
}

type AnnouncementAdmin struct {
	Announcement
	TelegramMode   string `json:"telegramMode"`
	TelegramText   string `json:"telegramText"`
	TelegramStatus string `json:"telegramStatus"`
	TelegramError  string `json:"telegramError"`
}

type AnnouncementSettings struct {
	Announcements []AnnouncementAdmin `json:"announcements"`
	Channel       string              `json:"channel"`
	Revision      string              `json:"revision"`
}

func parseAnnouncements(raw string) ([]Announcement, error) {
	items := []Announcement{}
	if strings.TrimSpace(raw) != "" {
		var records []map[string]any
		if err := common.Unmarshal([]byte(raw), &records); err != nil {
			return nil, err
		}
		for _, record := range records {
			if id, ok := record["id"].(float64); ok {
				record["id"] = strconv.FormatFloat(id, 'f', -1, 64)
			}
			b, err := common.Marshal(record)
			if err != nil {
				return nil, err
			}
			var item Announcement
			if err := common.Unmarshal(b, &item); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	for i := range items {
		if items[i].ID == "" {
			b, _ := common.Marshal(items[i])
			items[i].ID = fmt.Sprintf("legacy-%x", sha256.Sum256(append(b, []byte(fmt.Sprint(i))...)))[:39]
		}
	}
	return items, nil
}

// PublicAnnouncements only reads the existing console snapshot. The caller may
// already hold OptionMapRWMutex, so this function must not acquire it again.
func PublicAnnouncements() ([]Announcement, error) {
	items, err := parseAnnouncements(console_setting.GetConsoleSetting().Announcements)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].TelegramURL != "" {
			if _, _, err := console_setting.ParseTelegramPostURL(items[i].TelegramURL); err != nil {
				items[i].TelegramURL = ""
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339, items[i].PublishDate)
		right, _ := time.Parse(time.RFC3339, items[j].PublishDate)
		return left.After(right)
	})
	return items, nil
}

func getAnnouncementOptions(tx *gorm.DB) (string, string, error) {
	var raw, channel string
	for _, key := range []string{announcementsOption, telegramChannelOption} {
		option := Option{Key: key}
		if key == announcementsOption {
			var legacy Option
			err := tx.Where(map[string]any{"key": "Announcements"}).First(&legacy).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return "", "", err
			}
			option.Value = legacy.Value
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
			return "", "", err
		}
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(map[string]any{"key": key}).First(&option).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", err
		}
		if key == announcementsOption {
			raw = option.Value
		} else {
			channel = option.Value
		}
	}
	return raw, channel, nil
}

func announcementSettingsTx(tx *gorm.DB, raw, channel string) (AnnouncementSettings, error) {
	items, err := parseAnnouncements(raw)
	if err != nil {
		return AnnouncementSettings{}, err
	}
	var deliveries []AnnouncementTelegramDelivery
	if err := tx.Order("announcement_id").Find(&deliveries).Error; err != nil {
		return AnnouncementSettings{}, err
	}
	byID := make(map[string]AnnouncementTelegramDelivery, len(deliveries))
	for _, d := range deliveries {
		byID[d.AnnouncementID] = d
	}
	settings := AnnouncementSettings{Channel: channel, Announcements: make([]AnnouncementAdmin, 0, len(items))}
	for _, item := range items {
		d := byID[item.ID]
		mode := d.Mode
		if mode == "" {
			mode = "platform"
			if item.TelegramURL != "" {
				mode = "link"
			}
		}
		settings.Announcements = append(settings.Announcements, AnnouncementAdmin{Announcement: item, TelegramMode: mode, TelegramText: d.DesiredText, TelegramStatus: d.Status, TelegramError: d.Error})
	}
	// Delivery progress is deliberately excluded: only editable state conflicts.
	editable := make([]any, 0, len(settings.Announcements))
	for _, item := range settings.Announcements {
		editable = append(editable, []string{item.ID, item.TelegramMode, item.TelegramText})
	}
	b, _ := common.Marshal([]any{raw, channel, editable})
	settings.Revision = fmt.Sprintf("%x", sha256.Sum256(b))
	return settings, nil
}

func GetAnnouncementSettings() (AnnouncementSettings, error) {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	var result AnnouncementSettings
	var raw, channel string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		raw, channel, err = getAnnouncementOptions(tx)
		if err != nil {
			return err
		}
		result, err = announcementSettingsTx(tx, raw, channel)
		return err
	})
	if err == nil {
		err = refreshAnnouncementOptions(raw, channel)
	}
	return result, err
}

func SaveAnnouncementSettings(settings AnnouncementSettings) (AnnouncementSettings, error) {
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	settings.Channel = strings.TrimSpace(settings.Channel)
	if settings.Channel != "" {
		if err := console_setting.ValidateTelegramChannel(settings.Channel); err != nil {
			return AnnouncementSettings{}, err
		}
	}
	var saved AnnouncementSettings
	var raw string
	err := DB.Transaction(func(tx *gorm.DB) error {
		oldRaw, channel, err := getAnnouncementOptions(tx)
		if err != nil {
			return err
		}
		current, err := announcementSettingsTx(tx, oldRaw, channel)
		if err != nil {
			return err
		}
		if settings.Revision != current.Revision {
			return ErrAnnouncementRevisionConflict
		}
		public := make([]Announcement, 0, len(settings.Announcements))
		seen := make(map[string]bool)
		for _, item := range settings.Announcements {
			if item.ID == "" {
				item.ID = common.GetRandomString(32)
			}
			if !announcementIDPattern.MatchString(item.ID) || seen[item.ID] {
				return errors.New("announcement IDs must be unique and valid")
			}
			seen[item.ID] = true
			if item.Type == "" {
				item.Type = "default"
			}
			if err := saveAnnouncementDeliveryTx(tx, &item, settings.Channel); err != nil {
				return err
			}
			public = append(public, item.Announcement)
		}
		b, err := common.Marshal(public)
		if err != nil {
			return err
		}
		raw = string(b)
		if err := console_setting.ValidateConsoleSettings(raw, "Announcements"); err != nil {
			return err
		}
		var deliveries []AnnouncementTelegramDelivery
		if err := tx.Find(&deliveries).Error; err != nil {
			return err
		}
		for _, d := range deliveries {
			if seen[d.AnnouncementID] {
				continue
			}
			updates := map[string]any{"deleted": true}
			if d.Status != "sending" {
				updates["status"] = "cancelled"
			}
			if err := tx.Model(&d).Updates(updates).Error; err != nil {
				return err
			}
		}
		for key, value := range map[string]string{announcementsOption: raw, telegramChannelOption: settings.Channel} {
			if err := tx.Model(&Option{}).Where(map[string]any{"key": key}).Update("value", value).Error; err != nil {
				return err
			}
		}
		saved, err = announcementSettingsTx(tx, raw, settings.Channel)
		return err
	})
	if err != nil {
		return AnnouncementSettings{}, err
	}
	if err := refreshAnnouncementOptions(raw, settings.Channel); err != nil {
		return AnnouncementSettings{}, err
	}
	return saved, nil
}

func refreshAnnouncementOptions(raw, channel string) error {
	if err := updateOptionMapFromDatabase(announcementsOption, raw); err != nil {
		return err
	}
	return updateOptionMapFromDatabase(telegramChannelOption, channel)
}

// Legacy option clients may edit platform content, but cannot submit private
// Telegram controls or overwrite a bot-owned publication URL.
func normalizeLegacyAnnouncementsTx(tx *gorm.DB, raw string) (string, error) {
	if _, _, err := getAnnouncementOptions(tx); err != nil {
		return "", err
	}
	var records []map[string]any
	if strings.TrimSpace(raw) != "" {
		if err := common.Unmarshal([]byte(raw), &records); err != nil {
			return "", err
		}
	}
	for _, record := range records {
		for key := range record {
			if strings.HasPrefix(strings.ToLower(key), "telegram") && key != "telegramUrl" {
				return "", errors.New("Telegram controls must be saved through the announcements API")
			}
		}
	}
	items, err := parseAnnouncements(raw)
	if err != nil {
		return "", err
	}
	seen := make(map[string]bool, len(items))
	for i := range items {
		if !announcementIDPattern.MatchString(items[i].ID) || seen[items[i].ID] {
			return "", errors.New("announcement IDs must be unique and valid")
		}
		seen[items[i].ID] = true
		if items[i].TelegramURL != "" {
			if _, _, err := console_setting.ParseTelegramPostURL(items[i].TelegramURL); err != nil {
				return "", err
			}
		}
	}
	if tx.Migrator().HasTable(&AnnouncementTelegramDelivery{}) {
		var deliveries []AnnouncementTelegramDelivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Find(&deliveries).Error; err != nil {
			return "", err
		}
		for _, d := range deliveries {
			if !seen[d.AnnouncementID] {
				updates := map[string]any{"deleted": true}
				if d.Status != "sending" {
					updates["status"] = "cancelled"
				}
				if err := tx.Model(&d).Updates(updates).Error; err != nil {
					return "", err
				}
				continue
			}
			if d.Deleted {
				return "", errors.New("deleted announcement IDs cannot be reused")
			}
			if d.Mode == "bot" {
				for i := range items {
					if items[i].ID == d.AnnouncementID {
						items[i].TelegramURL = d.TelegramURL
					}
				}
			}
		}
	}
	b, err := common.Marshal(items)
	if err != nil {
		return "", err
	}
	if err := console_setting.ValidateConsoleSettings(string(b), "Announcements"); err != nil {
		return "", err
	}
	return string(b), nil
}

func saveAnnouncementDeliveryTx(tx *gorm.DB, item *AnnouncementAdmin, channel string) error {
	var d AnnouncementTelegramDelivery
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("announcement_id = ?", item.ID).First(&d).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	d.AnnouncementID = item.ID
	if d.Deleted {
		return errors.New("deleted announcement IDs cannot be reused")
	}
	if item.TelegramMode == "" {
		item.TelegramMode = "platform"
	}
	switch item.TelegramMode {
	case "platform":
		item.TelegramURL = ""
	case "link":
		if _, _, err := console_setting.ParseTelegramPostURL(item.TelegramURL); err != nil {
			return err
		}
	case "bot":
		if err := console_setting.ValidateTelegramText(item.TelegramText); err != nil {
			return err
		}
		if d.Channel == "" || (d.MessageID == 0 && d.Status != "sending" && d.Status != "unknown") {
			if err := console_setting.ValidateTelegramChannel(channel); err != nil {
				return err
			}
			d.Channel = channel
		}
		if d.DesiredText != item.TelegramText || d.Mode != "bot" || d.Deleted {
			d.DesiredVersion++
			d.DesiredText = item.TelegramText
			if d.Status != "sending" && d.Status != "unknown" {
				d.Status = "pending"
				d.Error = ""
			}
		}
		item.TelegramURL = d.TelegramURL
	default:
		return errors.New("invalid Telegram announcement mode")
	}
	if item.TelegramMode != "bot" {
		if d.Status != "sending" && d.Status != "unknown" {
			d.Status = "cancelled"
		}
	}
	d.Mode = item.TelegramMode
	d.Deleted = false
	return tx.Save(&d).Error
}
