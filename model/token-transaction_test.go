package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenReadUsesTransactionForColdSchemaInspection(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(fmt.Sprintf("model_mapping_column_%t", migrated), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Token{}))
			token := Token{Key: "transaction-schema-test", ModelMapping: `{"alias":"model"}`}
			require.NoError(t, db.Create(&token).Error)
			if !migrated {
				require.NoError(t, db.Migrator().DropColumn(&Token{}, "model_mapping"))
			}
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(1)
			originalDB := DB
			DB = db
			t.Cleanup(func() {
				DB = originalDB
				require.NoError(t, connection.Close())
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var loaded Token
			err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return tokenReadDB(tx).First(&loaded, token.Id).Error
			})
			require.NoError(t, err, "schema inspection must use the transaction's sole database connection")
			assert.Equal(t, token.Key, loaded.Key)
			if migrated {
				assert.Equal(t, token.ModelMapping, loaded.ModelMapping)
			} else {
				assert.Empty(t, loaded.ModelMapping)
			}
		})
	}
}
