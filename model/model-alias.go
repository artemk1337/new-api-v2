package model

import (
	"database/sql/driver"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelAliases stores additional public names without duplicating model records.
type ModelAliases []string

const modelAliasesGuardOption = "_ModelAliasesGuard"

func (a ModelAliases) Value() (driver.Value, error) {
	data, err := common.Marshal(a)
	return string(data), err
}

func (a *ModelAliases) Scan(value any) error {
	switch value := value.(type) {
	case nil:
		*a = nil
		return nil
	case string:
		return common.Unmarshal([]byte(value), a)
	case []byte:
		return common.Unmarshal(value, a)
	default:
		return fmt.Errorf("invalid model aliases value %T", value)
	}
}

// BeforeSave serializes name ownership across replicas before reading conflicts.
// Partial status updates do not change names and use an empty model receiver.
func (mi *Model) BeforeSave(tx *gorm.DB) error {
	if mi.ModelName == "" {
		return nil
	}
	tx = tx.Session(&gorm.Session{NewDB: true})
	guard := Option{Key: modelAliasesGuardOption}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
		return err
	}
	// A no-op write locks the guard until commit on all supported databases,
	// including SQLite, which does not support SELECT FOR UPDATE.
	if err := tx.Model(&Option{}).Where(map[string]any{"key": guard.Key}).Update("value", gorm.Expr("value")).Error; err != nil {
		return err
	}
	return mi.validateAliases(tx)
}

func (mi *Model) validateAliases(tx *gorm.DB) error {
	mi.ModelName = strings.TrimSpace(mi.ModelName)
	if mi.ModelName == "" {
		return fmt.Errorf("model name is required")
	}
	mi.Aliases = normalizeLookupValues(mi.Aliases)
	if mi.NameRule != NameRuleExact && len(mi.Aliases) > 0 {
		return fmt.Errorf("aliases require an exact model name")
	}
	names := map[string]bool{mi.ModelName: true}
	for _, alias := range mi.Aliases {
		if alias == mi.ModelName || len(alias) > 128 || strings.ContainsFunc(alias, unicode.IsSpace) {
			return fmt.Errorf("invalid model alias %q", alias)
		}
		names[alias] = true
	}
	var models []Model
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id <> ?", mi.Id).Find(&models).Error; err != nil {
		return err
	}
	for _, existing := range models {
		if existing.NameRule == NameRuleExact && names[existing.ModelName] {
			return fmt.Errorf("model name or alias %q already exists", existing.ModelName)
		}
		for _, alias := range existing.Aliases {
			if names[alias] {
				return fmt.Errorf("model name or alias %q already exists", alias)
			}
		}
	}
	return nil
}

var modelAliasCache struct {
	sync.Mutex
	db      *gorm.DB
	expires time.Time
	aliases map[string]string
}

func invalidateModelAliases() {
	modelAliasCache.Lock()
	defer modelAliasCache.Unlock()
	modelAliasCache.expires = time.Time{}
}

// GetModelAliases returns a snapshot; disabled models never expose aliases.
func GetModelAliases() map[string]string {
	modelAliasCache.Lock()
	defer modelAliasCache.Unlock()
	if modelAliasCache.db != DB || time.Now().After(modelAliasCache.expires) {
		var models []Model
		aliases := make(map[string]string)
		if err := DB.Find(&models).Error; err == nil {
			for _, m := range models {
				if m.NameRule == NameRuleExact && m.Status == 1 {
					for _, alias := range m.Aliases {
						aliases[alias] = m.ModelName
					}
				}
			}
		}
		modelAliasCache.db = DB
		modelAliasCache.aliases = aliases
		modelAliasCache.expires = time.Now().Add(time.Minute)
	}
	aliases := make(map[string]string, len(modelAliasCache.aliases))
	for alias, canonical := range modelAliasCache.aliases {
		aliases[alias] = canonical
	}
	return aliases
}

// AppendModelAliases exposes only aliases whose canonical model is available.
func AppendModelAliases(names []string) []string {
	available := make(map[string]bool, len(names))
	for _, name := range names {
		available[name] = true
	}
	var aliases []string
	for alias, canonical := range GetModelAliases() {
		if available[canonical] && !available[alias] {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return append(names, aliases...)
}
