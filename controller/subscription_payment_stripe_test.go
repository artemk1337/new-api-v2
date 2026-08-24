package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

func TestValidateStripeSubscriptionPriceRejectsRecurringPrices(t *testing.T) {
	previousLookup := stripeSubscriptionPriceLookup
	t.Cleanup(func() { stripeSubscriptionPriceLookup = previousLookup })
	stripeSubscriptionPriceLookup = func(string) (*stripe.Price, error) {
		return &stripe.Price{Type: stripe.PriceTypeRecurring, Recurring: &stripe.PriceRecurring{}}, nil
	}
	require.Error(t, validateStripeSubscriptionPrice("price_recurring", 10, "USD"))

	stripeSubscriptionPriceLookup = func(string) (*stripe.Price, error) {
		return &stripe.Price{Type: stripe.PriceTypeOneTime, Currency: stripe.CurrencyUSD, UnitAmount: 1000}, nil
	}
	require.NoError(t, validateStripeSubscriptionPrice("price_one_time", 10, "USD"))
	stripeSubscriptionPriceLookup = func(string) (*stripe.Price, error) {
		return &stripe.Price{Type: stripe.PriceTypeOneTime, Currency: stripe.CurrencyUSD, UnitAmount: 900}, nil
	}
	require.Error(t, validateStripeSubscriptionPrice("price_wrong_amount", 10, "USD"))
	stripeSubscriptionPriceLookup = func(string) (*stripe.Price, error) {
		return &stripe.Price{Type: stripe.PriceTypeOneTime, Currency: stripe.CurrencyUSD, UnitAmount: 1001}, nil
	}
	require.Error(t, validateStripeSubscriptionPrice("price_rounded_amount", 10.005, "USD"))
	require.Equal(t, stripe.CheckoutSessionModePayment, stripeSubscriptionCheckoutMode)
	require.NotEqual(t, stripe.CheckoutSessionModeSubscription, stripeSubscriptionCheckoutMode)
}

func setupStripeSubscriptionOrderDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
}

func TestIsPermanentStripeCreateError(t *testing.T) {
	require.True(t, isPermanentStripeCreateError(&stripe.Error{HTTPStatusCode: 400}))
	require.False(t, isPermanentStripeCreateError(&stripe.Error{HTTPStatusCode: 408}))
	require.False(t, isPermanentStripeCreateError(&stripe.Error{HTTPStatusCode: 429}))
	require.False(t, isPermanentStripeCreateError(&stripe.Error{HTTPStatusCode: 500}))
	require.False(t, isPermanentStripeCreateError(errors.New("transport timeout")))
}

func TestCreateStripeSubscriptionCheckoutPersistsPendingBeforeProviderCall(t *testing.T) {
	setupStripeSubscriptionOrderDB(t)
	previousCreator := stripeSubscriptionLinkCreator
	t.Cleanup(func() { stripeSubscriptionLinkCreator = previousCreator })

	const tradeNo = "sub-stripe-order-before-checkout"
	providerErr := errors.New("stripe transport timeout")
	providerCalled := false
	stripeSubscriptionLinkCreator = func(referenceId string, customerId string, email string, priceId string) (string, error) {
		providerCalled = true
		persisted := model.GetSubscriptionOrderByTradeNo(referenceId)
		require.NotNil(t, persisted)
		require.Equal(t, common.TopUpStatusPending, persisted.Status)
		return "", providerErr
	}

	order := &model.SubscriptionOrder{
		UserId:          41,
		PlanId:          7,
		Money:           12.50,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}
	user := &model.User{Id: 41, Email: "buyer@example.test"}
	plan := &model.SubscriptionPlan{Id: 7, StripePriceId: "price_test"}

	_, err := createStripeSubscriptionCheckout(order, user, plan)
	require.ErrorIs(t, err, providerErr)
	require.True(t, providerCalled)

	persisted := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, persisted)
	require.Equal(t, common.TopUpStatusPending, persisted.Status, "ambiguous provider errors must remain reconcilable")
	require.NotEmpty(t, persisted.PlanSnapshot)
}
