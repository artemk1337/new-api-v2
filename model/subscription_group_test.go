package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionUpgradeGroupStaysUserGroupName(t *testing.T) {
	truncateTables(t)

	user := &User{
		Id:       4101,
		Username: "subscription-user-group",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(user).Error)

	plan := &SubscriptionPlan{
		Id:            4101,
		Title:         "User Group Plan",
		PriceAmount:   1,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		UpgradeGroup:  "paid-user-group",
	}
	plan.NormalizeDefaults()
	assert.Equal(t, "paid-user-group", plan.UpgradeGroup)
	require.NoError(t, DB.Create(plan).Error)

	_, err := CreateUserSubscriptionFromPlanTx(DB, user.Id, plan, "test")
	require.NoError(t, err)

	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, "paid-user-group", group)
}
