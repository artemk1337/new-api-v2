package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelAliasesPersistenceAndConflicts(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	t.Cleanup(func() {
		DB.Unscoped().Where("model_name LIKE ?", "alias-test-%").Delete(&Model{})
		invalidateModelAliases()
	})
	m := Model{ModelName: "alias-test-canonical", Aliases: ModelAliases{" alias-test-cursor ", "alias-test-cursor", ""}, Status: 1}
	require.NoError(t, m.Insert())
	require.Equal(t, ModelAliases{"alias-test-cursor"}, m.Aliases)
	var loaded Model
	require.NoError(t, DB.First(&loaded, m.Id).Error)
	require.Equal(t, m.Aliases, loaded.Aliases)
	invalidateModelAliases()
	require.Equal(t, m.ModelName, GetModelAliases()["alias-test-cursor"])

	for _, conflict := range []Model{
		{ModelName: "alias-test-other", Aliases: ModelAliases{"alias-test-cursor"}},
		{ModelName: "alias-test-cursor"},
		{ModelName: "alias-test-other", Aliases: ModelAliases{"alias-test-canonical"}},
		{ModelName: "alias-test-other", Aliases: ModelAliases{"alias-test-other"}},
		{ModelName: "alias-test-other", Aliases: ModelAliases{"bad alias"}},
		{ModelName: "alias-test-rule", NameRule: NameRulePrefix, Aliases: ModelAliases{"alias-test-other"}},
	} {
		require.Error(t, conflict.Insert(), "%+v", conflict)
	}

	m.Aliases = ModelAliases{"alias-test-renamed"}
	require.NoError(t, m.Update())
	invalidateModelAliases()
	require.NotContains(t, GetModelAliases(), "alias-test-cursor")
	require.Equal(t, m.ModelName, GetModelAliases()["alias-test-renamed"])
	m.Status = 0
	require.NoError(t, m.Update())
	invalidateModelAliases()
	require.NotContains(t, GetModelAliases(), "alias-test-renamed")
	m.Aliases = nil
	require.NoError(t, m.Update())
	require.NoError(t, DB.First(&loaded, m.Id).Error)
	require.Empty(t, loaded.Aliases)
}

func TestModelAliasOwnershipIsAtomic(t *testing.T) {
	for _, create := range []bool{false, true} {
		name := "update"
		if create {
			name = "direct-create"
		}
		t.Run(name, func(t *testing.T) {
			previousDB := DB
			db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "aliases.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
			require.NoError(t, err)
			DB = db
			t.Cleanup(func() {
				DB = previousDB
				sqlDB, err := db.DB()
				if err == nil {
					_ = sqlDB.Close()
				}
			})
			require.NoError(t, db.AutoMigrate(&Model{}, &Option{}))
			candidates := []Model{{ModelName: "first-model", Status: 1}, {ModelName: "second-model", Status: 1}}
			if !create {
				for i := range candidates {
					require.NoError(t, candidates[i].Insert())
				}
			}
			start := make(chan struct{})
			results := make(chan error, len(candidates))
			for _, candidate := range candidates {
				go func(candidate Model) {
					<-start
					candidate.Aliases = ModelAliases{"shared-alias"}
					if create {
						results <- db.Create(&candidate).Error
					} else {
						results <- candidate.Update()
					}
				}(candidate)
			}
			close(start)
			successes := 0
			for range candidates {
				err := <-results
				if err == nil {
					successes++
				} else {
					require.ErrorContains(t, err, "already exists")
				}
			}
			require.Equal(t, 1, successes)
			var saved []Model
			require.NoError(t, db.Find(&saved).Error)
			owners := 0
			for _, m := range saved {
				if len(m.Aliases) > 0 {
					owners++
				}
			}
			require.Equal(t, 1, owners)
			require.ErrorContains(t, db.Create(&Model{ModelName: "shared-alias"}).Error, "already exists")
			require.NoError(t, updateOptionMapFromDatabase(modelAliasesGuardOption, ""))
			require.NotContains(t, common.OptionMap, modelAliasesGuardOption)
		})
	}
}
