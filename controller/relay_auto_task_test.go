package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestApplyTaskSettlementBaselineUsesHeldReserve(t *testing.T) {
	task := &model.Task{
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{},
		},
	}
	settlementErr := errors.New("database unavailable")

	applyTaskSettlementBaseline(task, service.BillingSettlementOutcome{
		Quota:       500,
		Err:         settlementErr,
		HeldReserve: true,
	})

	require.Equal(t, 500, task.Quota)
	require.Equal(t, settlementErr.Error(), task.PrivateData.BillingContext.SettlementError)
	require.True(t, task.PrivateData.BillingContext.HeldReserve)
}

func TestApplyTaskFundingSnapshotPersistsSubscriptionWalletSplit(t *testing.T) {
	task := &model.Task{}
	info := &relaycommon.RelayInfo{
		BillingSource:        service.BillingSourceSubscription,
		BillingOverageSource: service.BillingSourceWallet,
		BillingOverageQuota:  350,
		SubscriptionId:       41,
		TokenId:              42,
	}

	applyTaskFundingSnapshot(task, info)

	require.Equal(t, service.BillingSourceSubscription, task.PrivateData.BillingSource)
	require.Equal(t, service.BillingSourceWallet, task.PrivateData.BillingOverageSource)
	require.Equal(t, 350, task.PrivateData.BillingOverageQuota)
	require.Equal(t, 41, task.PrivateData.SubscriptionId)
	require.Equal(t, 42, task.PrivateData.TokenId)
}

func TestValidateAutoTaskPricingContractRejectsMixedAdaptors(t *testing.T) {
	priority := int64(10)
	groups := []string{"1", "2"}
	abilities := []model.AbilityWithChannel{
		{
			Ability: model.Ability{
				Group:    "1",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType: constant.ChannelTypeSora,
		},
		{
			Ability: model.Ability{
				Group:    "2",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType: constant.ChannelTypeGemini,
		},
	}

	err := validateAutoTaskPricingContractWithAbilities(groups, "task-model", "", abilities)

	require.ErrorContains(t, err, "same task pricing contract")
}

func TestValidateAutoTaskPricingContractAllowsEquivalentChannelTypes(t *testing.T) {
	priority := int64(10)
	groups := []string{"1", "2"}
	abilities := []model.AbilityWithChannel{
		{
			Ability: model.Ability{
				Group:    "1",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType: constant.ChannelTypeSora,
		},
		{
			Ability: model.Ability{
				Group:    "2",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}

	require.NoError(t, validateAutoTaskPricingContractWithAbilities(groups, "task-model", "", abilities))
}

func TestValidateAutoTaskPricingContractRejectsDifferentMappedModels(t *testing.T) {
	priority := int64(10)
	firstMapping := `{"task-model":"upstream-a"}`
	secondMapping := `{"task-model":"upstream-b"}`
	groups := []string{"1", "2"}
	abilities := []model.AbilityWithChannel{
		{
			Ability: model.Ability{
				Group:    "1",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType:         constant.ChannelTypeSora,
			ChannelModelMapping: &firstMapping,
		},
		{
			Ability: model.Ability{
				Group:    "2",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: &secondMapping,
		},
	}

	err := validateAutoTaskPricingContractWithAbilities(groups, "task-model", "", abilities)

	require.ErrorContains(t, err, "same mapped upstream model")
}

func TestValidateAutoTaskPricingContractAllowsEquivalentMappedModels(t *testing.T) {
	priority := int64(10)
	firstMapping := `{"task-model":"intermediate","intermediate":"upstream"}`
	secondMapping := `{"task-model":"upstream"}`
	groups := []string{"1", "2"}
	abilities := []model.AbilityWithChannel{
		{
			Ability: model.Ability{
				Group:    "1",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType:         constant.ChannelTypeSora,
			ChannelModelMapping: &firstMapping,
		},
		{
			Ability: model.Ability{
				Group:    "2",
				Model:    "task-model",
				Priority: &priority,
			},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: &secondMapping,
		},
	}

	require.NoError(t, validateAutoTaskPricingContractWithAbilities(groups, "task-model", "", abilities))
}
