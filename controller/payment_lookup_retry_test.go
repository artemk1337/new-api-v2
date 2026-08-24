package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCompleteNOWPaymentsPaymentRetriesTopUpDatabaseFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	statusCode, err := completeNOWPaymentsPayment(&service.NOWPaymentsPayment{
		PaymentStatus: "finished",
		OrderID:       "now-db-error",
		PriceAmount:   "1.00",
		PriceCurrency: "USD",
	}, "127.0.0.1")
	require.Error(t, err)
	require.Equal(t, http.StatusInternalServerError, statusCode)
}
