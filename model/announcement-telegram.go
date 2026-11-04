package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnouncementTelegramDelivery struct {
	AnnouncementID   string `gorm:"type:varchar(64);primaryKey"`
	Mode             string `gorm:"type:varchar(16)"`
	DesiredVersion   uint64
	DeliveredVersion uint64
	DesiredText      string `gorm:"type:text"`
	DeliveredText    string `gorm:"type:text"`
	Channel          string `gorm:"type:varchar(64)"`
	ChatID           int64
	MessageID        int
	TelegramURL      string `gorm:"type:varchar(256)"`
	Status           string `gorm:"type:varchar(16);index"`
	Error            string `gorm:"type:text"`
	AttemptID        string `gorm:"type:varchar(64)"`
	AttemptVersion   uint64
	AttemptText      string `gorm:"type:text"`
	AttemptOwner     string `gorm:"type:varchar(64)"`
	AttemptStartedAt int64
	Deleted          bool
}

type AnnouncementTelegramResult struct {
	ChatID      int64
	MessageID   int
	TelegramURL string
	Status      string
	Error       string
}

func HasAnnouncementTelegramWork() (bool, error) {
	var count int64
	err := DB.Model(&AnnouncementTelegramDelivery{}).Where("status = ? OR (status = ? AND mode = ? AND deleted = ?)", "sending", "pending", "bot", false).Count(&count).Error
	return count > 0, err
}

// The caller owns the system-task type lease before recovery. An interrupted
// create cannot be retried safely; editing a known message is idempotent.
func RecoverAnnouncementTelegramDeliveries(owner string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&AnnouncementTelegramDelivery{}).Where("status = ? AND attempt_owner <> ? AND (deleted = ? OR (mode <> ? AND message_id > 0))", "sending", owner, true, "bot").Update("status", "cancelled").Error; err != nil {
			return err
		}
		query := tx.Model(&AnnouncementTelegramDelivery{}).Where("status = ? AND attempt_owner <> ?", "sending", owner)
		if err := query.Where("message_id = 0").Updates(map[string]any{"status": "unknown", "error": "Telegram publication outcome is unknown; check the channel before retrying"}).Error; err != nil {
			return err
		}
		return tx.Model(&AnnouncementTelegramDelivery{}).Where("status = ? AND attempt_owner <> ? AND message_id > 0", "sending", owner).Updates(map[string]any{"status": "pending", "error": ""}).Error
	})
}

func ClaimAnnouncementTelegramDelivery(owner string) (AnnouncementTelegramDelivery, bool, error) {
	var d AnnouncementTelegramDelivery
	claimed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status = ? AND mode = ? AND deleted = ?", "pending", "bot", false).Order("announcement_id").First(&d).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		d.AttemptID = common.GetRandomString(32)
		d.AttemptOwner = owner
		d.AttemptVersion = d.DesiredVersion
		d.AttemptText = d.DesiredText
		d.AttemptStartedAt = time.Now().Unix()
		d.Status = "sending"
		result := tx.Model(&AnnouncementTelegramDelivery{}).Where("announcement_id = ? AND status = ?", d.AnnouncementID, "pending").Updates(map[string]any{"status": d.Status, "attempt_id": d.AttemptID, "attempt_owner": owner, "attempt_version": d.AttemptVersion, "attempt_text": d.AttemptText, "attempt_started_at": d.AttemptStartedAt})
		claimed = result.RowsAffected == 1
		return result.Error
	})
	return d, claimed, err
}

func FinishAnnouncementTelegramDelivery(attempt AnnouncementTelegramDelivery, result AnnouncementTelegramResult) error {
	if result.Status != "sent" && result.Status != "failed" && result.Status != "unknown" {
		return errors.New("invalid Telegram result status")
	}
	optionUpdateMutex.Lock()
	defer optionUpdateMutex.Unlock()
	var raw, channel string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		raw, channel, err = getAnnouncementOptions(tx)
		if err != nil {
			return err
		}
		var d AnnouncementTelegramDelivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("announcement_id = ?", attempt.AnnouncementID).First(&d).Error; err != nil {
			return err
		}
		if d.Status != "sending" || d.AttemptID != attempt.AttemptID || d.AttemptOwner != attempt.AttemptOwner {
			return errors.New("Telegram delivery attempt is no longer current")
		}
		d.Status = result.Status
		d.Error = result.Error
		if result.Status == "sent" {
			if result.MessageID <= 0 {
				return errors.New("Telegram response has no message ID")
			}
			if result.TelegramURL == "" {
				result.TelegramURL = fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(d.Channel, "@"), result.MessageID)
			}
			urlChannel, messageID, err := console_setting.ParseTelegramPostURL(result.TelegramURL)
			if err != nil || !strings.EqualFold(urlChannel, d.Channel) || messageID != result.MessageID {
				return errors.New("Telegram response has an invalid post URL")
			}
			d.ChatID = result.ChatID
			d.MessageID = result.MessageID
			d.TelegramURL = result.TelegramURL
			d.DeliveredVersion = d.AttemptVersion
			d.DeliveredText = d.AttemptText
			d.Error = ""
			if d.Mode == "bot" && !d.Deleted && d.DesiredVersion > d.DeliveredVersion {
				d.Status = "pending"
			}
			items, err := parseAnnouncements(raw)
			if err != nil {
				return err
			}
			for i := range items {
				if items[i].ID == d.AnnouncementID && d.Mode == "bot" && !d.Deleted {
					items[i].TelegramURL = d.TelegramURL
				}
			}
			b, err := common.Marshal(items)
			if err != nil {
				return err
			}
			raw = string(b)
			if err := tx.Model(&Option{}).Where(map[string]any{"key": announcementsOption}).Update("value", raw).Error; err != nil {
				return err
			}
		}
		if d.Deleted || (d.Mode != "bot" && d.Status != "unknown") {
			d.Status = "cancelled"
		}
		return tx.Save(&d).Error
	})
	if err != nil {
		return err
	}
	return refreshAnnouncementOptions(raw, channel)
}

func RetryAnnouncementTelegramDelivery(id string, allowUnknown bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var d AnnouncementTelegramDelivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("announcement_id = ?", id).First(&d).Error; err != nil {
			return err
		}
		if d.Deleted || d.Mode != "bot" {
			return errors.New("announcement is not configured for Telegram publication")
		}
		if d.Status == "unknown" && !allowUnknown {
			return errors.New("confirm the unknown publication result before retrying")
		}
		if d.Status != "failed" && d.Status != "unknown" {
			return errors.New("only failed or unknown Telegram publications can be retried")
		}
		return tx.Model(&d).Where("status = ? AND attempt_id = ?", d.Status, d.AttemptID).Updates(map[string]any{"status": "pending", "error": ""}).Error
	})
}
