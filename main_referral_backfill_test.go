package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestBackfillReferralCashbackEligibilitySkipsReplica(t *testing.T) {
	wasMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = wasMaster })

	require.NoError(t, backfillReferralCashbackEligibilityOnMaster())
}
