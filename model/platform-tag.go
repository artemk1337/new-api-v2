package model

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformTag stores tags created by administrators independently of their use.
type PlatformTag struct {
	Id   int    `json:"id"`
	Name string `json:"name" gorm:"size:191;not null;uniqueIndex"`
}

func GetPlatformTags() ([]string, error) {
	tags := make([]string, 0)
	err := DB.Model(&PlatformTag{}).Order("name").Pluck("name", &tags).Error
	return tags, err
}

func CreatePlatformTag(name string) (string, error) {
	tag := PlatformTag{Name: name}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&tag).Error; err != nil {
		return "", err
	}
	if err := DB.Where("name = ?", name).First(&tag).Error; err != nil {
		return "", err
	}
	return tag.Name, nil
}

func registerPlatformTagTx(tx *gorm.DB, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if utf8.RuneCountInString(name) > 191 {
		return errors.New("tag cannot exceed 191 characters")
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&PlatformTag{Name: name}).Error
}

func registerPricingGroupTagsTx(tx *gorm.DB, value string) error {
	if strings.HasPrefix(strings.TrimSpace(value), "{") {
		return nil // Legacy GroupRatio maps do not contain tags.
	}
	var groups []ratio_setting.PricingGroup
	if err := common.Unmarshal([]byte(value), &groups); err != nil {
		return err
	}
	for _, group := range groups {
		if err := registerPlatformTagTx(tx, group.Tag); err != nil {
			return err
		}
	}
	return nil
}

// Backfill existing values without changing channel or pricing group storage.
func migratePlatformTags() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var tags []string
		if err := tx.Model(&Channel{}).Distinct("tag").Where("tag IS NOT NULL AND tag <> ?", "").Pluck("tag", &tags).Error; err != nil {
			return err
		}
		for _, tag := range tags {
			if utf8.RuneCountInString(strings.TrimSpace(tag)) <= 191 {
				if err := registerPlatformTagTx(tx, tag); err != nil {
					return err
				}
			}
		}
		var option Option
		err := tx.Where(map[string]any{"key": "PricingGroups"}).First(&option).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var groups []ratio_setting.PricingGroup
		if err := common.Unmarshal([]byte(option.Value), &groups); err != nil {
			return nil // Legacy group-ratio maps have no tag fields.
		}
		for _, group := range groups {
			if utf8.RuneCountInString(strings.TrimSpace(group.Tag)) <= 191 {
				if err := registerPlatformTagTx(tx, group.Tag); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
