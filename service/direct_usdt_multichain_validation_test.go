package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidDirectUSDTTONTransferFinalityAndGuards(t *testing.T) {
	now := time.Now().Unix()
	owner := setting.USDTTONJettonMaster
	// Contract hash is not an owner; use a syntactically valid workchain-0 raw address.
	owner = "0:" + "1111111111111111111111111111111111111111111111111111111111111111"
	transfer := TONJettonTransfer{TransactionHash: "abc", LT: "42", Utime: now, Amount: "10000000", Destination: owner, JettonMaster: setting.USDTTONJettonMaster, FinalityProof: true}
	require.True(t, ValidDirectUSDTTONTransfer(transfer, owner, 10000000, now))
	transfer.TransactionAborted = true
	require.False(t, ValidDirectUSDTTONTransfer(transfer, owner, 10000000, now))
}

func TestValidDirectUSDTSolanaTransferRejectsFailedTx(t *testing.T) {
	now := time.Now().Unix()
	bt := now
	transfer := SolanaTransfer{Signature: "sig", InstructionIndex: 0, BlockTime: &bt, Mint: setting.USDTSolanaMint, Program: setting.USDTSolanaTokenProgram, Destination: "11111111111111111111111111111111", Amount: 10, Decimals: 6, MetaErr: true}
	require.False(t, ValidDirectUSDTSolanaTransfer(transfer, transfer.Destination, 10, now))
}

func TestValidDirectUSDTSolanaTransferAcceptsReconciliationWindowBoundary(t *testing.T) {
	now := int64(1_000_000)
	blockTime := now - int64(directUSDTReconciliationOverlap/time.Second)
	transfer := SolanaTransfer{
		Signature:        "sig",
		InstructionIndex: 0,
		BlockTime:        &blockTime,
		Mint:             setting.USDTSolanaMint,
		Program:          setting.USDTSolanaTokenProgram,
		Destination:      "11111111111111111111111111111111",
		Amount:           10,
		Decimals:         6,
	}
	require.True(t, ValidDirectUSDTSolanaTransfer(transfer, transfer.Destination, 10, now))
}

func TestSolanaExtractTransfersAcceptsOrdinarySPLTransferWithFinalizedDestinationMetadata(t *testing.T) {
	now := time.Now().Unix()
	destination := "11111111111111111111111111111111"
	transaction := map[string]any{
		"transaction": map[string]any{
			"message": map[string]any{
				"accountKeys": []any{"source", destination},
				"instructions": []any{map[string]any{
					"programId": setting.USDTSolanaTokenProgram,
					"parsed":    map[string]any{"type": "transfer", "info": map[string]any{"destination": destination, "amount": "10000000"}},
				}},
			},
		},
		"meta": map[string]any{
			"err": nil,
			"postTokenBalances": []any{map[string]any{
				"accountIndex":  1,
				"mint":          setting.USDTSolanaMint,
				"uiTokenAmount": map[string]any{"decimals": 6},
			}},
		},
	}

	transfers := solanaExtractTransfers(transaction, "signature", &now)
	require.Len(t, transfers, 1)
	require.True(t, ValidDirectUSDTSolanaTransfer(transfers[0], destination, 10000000, now))
}

func TestSolanaExtractTransfersRejectsOrdinaryTransferWithoutDestinationMintProof(t *testing.T) {
	now := time.Now().Unix()
	transaction := map[string]any{
		"transaction": map[string]any{"message": map[string]any{"accountKeys": []any{"source", "11111111111111111111111111111111"}, "instructions": []any{map[string]any{
			"programId": setting.USDTSolanaTokenProgram,
			"parsed":    map[string]any{"type": "transfer", "info": map[string]any{"destination": "11111111111111111111111111111111", "amount": "10000000"}},
		}}}},
		"meta": map[string]any{"err": nil},
	}
	require.Empty(t, solanaExtractTransfers(transaction, "signature", &now))
}

func TestSolanaExtractTransfersUsesStableOuterAndInnerInstructionPaths(t *testing.T) {
	now := time.Now().Unix()
	destination := "11111111111111111111111111111111"
	transferChecked := func(amount string) map[string]any {
		return map[string]any{
			"programId": setting.USDTSolanaTokenProgram,
			"parsed": map[string]any{
				"type": "transferChecked",
				"info": map[string]any{
					"destination": destination,
					"mint":        setting.USDTSolanaMint,
					"tokenAmount": map[string]any{"amount": amount, "decimals": 6},
				},
			},
		}
	}
	transaction := map[string]any{
		"transaction": map[string]any{
			"message": map[string]any{
				"instructions": []any{transferChecked("1")},
			},
		},
		"meta": map[string]any{
			"err": nil,
			"innerInstructions": []any{map[string]any{
				"index":        0,
				"instructions": []any{transferChecked("2"), transferChecked("3")},
			}},
		},
	}

	first := solanaExtractTransfers(transaction, "signature", &now)
	require.Len(t, first, 3)
	require.Equal(t, []string{"outer:0", "inner:0:0", "inner:0:1"}, []string{first[0].InstructionPath, first[1].InstructionPath, first[2].InstructionPath})
	second := solanaExtractTransfers(transaction, "signature", &now)
	require.Equal(t, []string{"outer:0", "inner:0:0", "inner:0:1"}, []string{second[0].InstructionPath, second[1].InstructionPath, second[2].InstructionPath})
}

func TestPollDirectUSDTTONOnceReconcilesHistoricalInvoiceWithoutCurrentAPIKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.DirectCryptoPayment{}))
	now := time.Now().Unix()
	address := "0:" + strings.Repeat("1", 64)
	require.NoError(t, db.Create(&model.DirectCryptoPayment{
		TradeNo: "ton-key-rotation", UserId: 1, Network: "TON", Token: "USDT",
		Contract: setting.USDTTONJettonMaster, Address: address, ReceivingOwner: address,
		Destination: address, ExpectedUnits: 10_000_001, BaseUnits: 10_000_000,
		SuffixUnits: 1, Status: model.DirectCryptoPending, CreatedAt: now, UpdatedAt: now,
		ExpiresAt: now + 60,
	}).Error)

	previousDB := model.DB
	previousKey := setting.USDTTONAPIKey
	previousBaseURL := setting.USDTTONAPIBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	model.DB = db
	setting.USDTTONAPIKey = ""
	setting.USDTTONAPIBaseURL = "https://toncenter.com/api/v3"
	var requests atomic.Int32
	DirectUSDTTRC20HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		require.Empty(t, req.Header.Get("X-API-Key"))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"jetton_transfers":[]}`)), Request: req}, nil
	})}
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTONAPIKey = previousKey
		setting.USDTTONAPIBaseURL = previousBaseURL
		DirectUSDTTRC20HTTPClient = previousClient
	})

	require.NoError(t, PollDirectUSDTTONOnce(context.Background()))
	require.Positive(t, requests.Load())
}
