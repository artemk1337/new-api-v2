package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

var directUSDTMultiWatcherMu sync.Mutex
var directUSDTMultiWatcherCancel context.CancelFunc

func StartDirectUSDTMultiChainWatcher() func() {
	directUSDTMultiWatcherMu.Lock()
	if directUSDTMultiWatcherCancel != nil {
		c := directUSDTMultiWatcherCancel
		directUSDTMultiWatcherMu.Unlock()
		return c
	}
	ctx, cancel := context.WithCancel(context.Background())
	directUSDTMultiWatcherCancel = cancel
	directUSDTMultiWatcherMu.Unlock()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var tonBackoff, solanaBackoff time.Duration
		for {
			tonBackoff = pollMultiChainNetwork(ctx, "TON", tonBackoff, PollDirectUSDTTONOnce)
			solanaBackoff = pollMultiChainNetwork(ctx, "SOLANA", solanaBackoff, PollDirectUSDTSolanaOnce)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

func pollMultiChainNetwork(ctx context.Context, network string, backoff time.Duration, poll func(context.Context) error) time.Duration {
	if err := poll(ctx); err != nil {
		common.SysLog(fmt.Sprintf("direct USDT %s watcher failed: %v", network, err))
		if backoff == 0 {
			backoff = 5 * time.Second
		} else {
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		}
		timer := time.NewTimer(backoff)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		return backoff
	}
	return 0
}

// TONJettonTransfer is the normalized subset returned by Toncenter v3.
// Amount is in the jetton's six-decimal smallest units and Utime is seconds.
type TONJettonTransfer struct {
	TransactionHash    string `json:"transaction_hash"`
	LT                 string `json:"transaction_lt"`
	Utime              int64  `json:"transaction_now"`
	Amount             string `json:"amount"`
	Destination        string `json:"destination"`
	JettonMaster       string `json:"jetton_master"`
	TransactionAborted bool   `json:"transaction_aborted"`
	TraceID            string `json:"trace_id"`
	Incomplete         bool   `json:"incomplete"`
	FinalityProof      bool   `json:"-"` // set only after an explicit traces finality check
}

type tonTransfersResponse struct {
	Transactions []TONJettonTransfer `json:"jetton_transfers"`
}

func fetchTONTransfers(ctx context.Context, owner string, now int64) ([]TONJettonTransfer, error) {
	base := strings.TrimRight(strings.TrimSpace(setting.USDTTONAPIBaseURL), "/")
	var all []TONJettonTransfer
	for page := 0; page < 100; page++ {
		u, err := url.Parse(base + "/jetton/transfers")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("jetton_master", setting.USDTTONJettonMaster)
		q.Set("owner_address", owner)
		q.Set("direction", "in")
		q.Set("limit", "100")
		q.Set("offset", strconv.Itoa(page*100))
		q.Set("start_utime", strconv.FormatInt(now-48*3600, 10))
		q.Set("end_utime", strconv.FormatInt(now, 10))
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if key := strings.TrimSpace(setting.USDTTONAPIKey); key != "" {
			req.Header.Set("X-API-Key", key)
		}
		resp, err := DirectUSDTTRC20HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, er := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if er != nil {
			return nil, er
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Toncenter returned HTTP %d", resp.StatusCode)
		}
		var payload tonTransfersResponse
		if er := common.Unmarshal(body, &payload); er != nil {
			return nil, er
		}
		all = append(all, payload.Transactions...)
		if len(payload.Transactions) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("Toncenter pagination limit reached")
}

func fetchTONTraceFinality(ctx context.Context, traceID, txHash, lt string) (bool, error) {
	u, err := url.Parse(strings.TrimRight(setting.USDTTONAPIBaseURL, "/") + "/traces")
	if err != nil {
		return false, err
	}
	q := u.Query()
	q.Set("trace_id", traceID)
	q.Set("include_actions", "false")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, err
	}
	if key := strings.TrimSpace(setting.USDTTONAPIKey); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := DirectUSDTTRC20HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("Toncenter trace returned HTTP %d", resp.StatusCode)
	}
	var root struct {
		Traces []struct {
			IsIncomplete bool `json:"is_incomplete"`
			Transactions map[string]struct {
				Hash     string `json:"hash"`
				LT       string `json:"lt"`
				Finality any    `json:"finality"`
			} `json:"transactions"`
		} `json:"traces"`
	}
	if err := common.Unmarshal(body, &root); err != nil {
		return false, err
	}
	for _, trace := range root.Traces {
		if trace.IsIncomplete {
			continue
		}
		for _, tx := range trace.Transactions {
			if tx.Hash == txHash && tx.LT == lt {
				return fmt.Sprint(tx.Finality) == "2" || strings.EqualFold(fmt.Sprint(tx.Finality), "finalized"), nil
			}
		}
	}
	return false, nil
}

func ValidDirectUSDTTONTransfer(t TONJettonTransfer, owner string, expected uint64, now int64) bool {
	canonical, err := setting.CanonicalTONAddress(owner)
	if err != nil {
		return false
	}
	destination, err := setting.CanonicalTONAddress(t.Destination)
	if err != nil || destination != canonical {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(t.JettonMaster), setting.USDTTONJettonMaster) || !t.FinalityProof || t.Incomplete || t.TransactionAborted || t.TransactionHash == "" || t.Utime <= 0 || t.Utime < now-48*3600 || t.Utime > now {
		return false
	}
	amount, err := strconv.ParseUint(strings.TrimSpace(t.Amount), 10, 64)
	return err == nil && amount == expected
}

// SolanaTransfer is a normalized legacy SPL transfer instruction extracted
// from a finalized jsonParsed transaction. Both transferChecked and ordinary
// transfer are supported; the latter obtains mint/decimals from the finalized
// post-token balance of the destination account. Destination must be a legacy
// SPL token account; Token-2022 program IDs are rejected by the validator.
type SolanaTransfer struct {
	Signature        string
	InstructionIndex int
	BlockTime        *int64
	Mint             string
	Program          string
	Destination      string
	Amount           uint64
	Decimals         uint8
	MetaErr          bool
}
type solanaSignatureInfo struct {
	Signature string `json:"signature"`
	BlockTime *int64 `json:"blockTime"`
	Err       any    `json:"err"`
}

func fetchSolanaSignatures(ctx context.Context, address string) ([]solanaSignatureInfo, error) {
	var all []solanaSignatureInfo
	before := ""
	for page := 0; page < 100; page++ {
		opts := map[string]any{"limit": 100, "commitment": "finalized"}
		if before != "" {
			opts["before"] = before
		}
		var batch []solanaSignatureInfo
		if err := solanaRPC(ctx, "getSignaturesForAddress", []any{address, opts}, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
		before = batch[len(batch)-1].Signature
	}
	return nil, fmt.Errorf("Solana signature pagination limit reached")
}

func solanaRPC(ctx context.Context, method string, params any, out any) error {
	body, err := common.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setting.USDTSolanaRPCURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(setting.USDTSolanaAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := DirectUSDTTRC20HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Solana RPC returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Result any `json:"result"`
		Error  any `json:"error"`
	}
	if err := common.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return fmt.Errorf("Solana RPC error")
	}
	b, _ := common.Marshal(envelope.Result)
	return common.Unmarshal(b, out)
}

func solanaExtractTransfers(value any, signature string, blockTime *int64) []SolanaTransfer {
	var result []SolanaTransfer
	tokenAccounts := solanaPostTokenBalances(value)
	var walk func(any)
	walk = func(node any) {
		m, ok := node.(map[string]any)
		if !ok {
			if a, ok := node.([]any); ok {
				for _, x := range a {
					walk(x)
				}
			}
			return
		}
		if parsed, ok := m["parsed"].(map[string]any); ok {
			typ, _ := parsed["type"].(string)
			info, infoOK := parsed["info"].(map[string]any)
			if infoOK && (typ == "transferChecked" || typ == "transfer") {
				destination := fmt.Sprint(info["destination"])
				amount := uint64(0)
				mint := ""
				decimals := uint8(0)
				validMetadata := false
				if typ == "transferChecked" {
					ta, ok := info["tokenAmount"].(map[string]any)
					if ok {
						parsedAmount, amountErr := strconv.ParseUint(fmt.Sprint(ta["amount"]), 10, 64)
						parsedDecimals, decimalsErr := strconv.ParseUint(fmt.Sprint(ta["decimals"]), 10, 8)
						if amountErr == nil && decimalsErr == nil {
							amount, mint, decimals, validMetadata = parsedAmount, fmt.Sprint(info["mint"]), uint8(parsedDecimals), true
						}
					}
				} else if balance, ok := tokenAccounts[destination]; ok {
					// SPL Token's ordinary transfer instruction does not carry mint or
					// decimals. Bind it to the finalized destination account balance;
					// do not infer either field from a client-controlled instruction.
					parsedAmount, amountErr := strconv.ParseUint(fmt.Sprint(info["amount"]), 10, 64)
					if amountErr == nil {
						amount, mint, decimals, validMetadata = parsedAmount, balance.mint, balance.decimals, true
					}
				}
				if validMetadata {
					result = append(result, SolanaTransfer{Signature: signature, InstructionIndex: len(result), BlockTime: blockTime, Mint: mint, Program: fmt.Sprint(m["programId"]), Destination: destination, Amount: amount, Decimals: decimals})
				}
			}
		}
		for _, x := range m {
			walk(x)
		}
	}
	walk(value)
	metaErr := false
	if root, ok := value.(map[string]any); ok {
		if meta, ok := root["meta"].(map[string]any); ok {
			if e, exists := meta["err"]; exists && e != nil {
				metaErr = true
			}
		}
	}
	if metaErr {
		for i := range result {
			result[i].MetaErr = true
		}
	}
	return result
}

type solanaTokenAccountBalance struct {
	mint     string
	decimals uint8
}

// solanaPostTokenBalances maps a destination token account to the mint and
// decimals proved by a finalized getTransaction response. Ordinary SPL
// transfer instructions do not include those fields themselves.
func solanaPostTokenBalances(value any) map[string]solanaTokenAccountBalance {
	balances := make(map[string]solanaTokenAccountBalance)
	root, ok := value.(map[string]any)
	if !ok {
		return balances
	}
	transaction, ok := root["transaction"].(map[string]any)
	if !ok {
		return balances
	}
	message, ok := transaction["message"].(map[string]any)
	if !ok {
		return balances
	}
	keys, ok := message["accountKeys"].([]any)
	if !ok {
		return balances
	}
	meta, ok := root["meta"].(map[string]any)
	if !ok {
		return balances
	}
	postBalances, ok := meta["postTokenBalances"].([]any)
	if !ok {
		return balances
	}
	for _, rawBalance := range postBalances {
		balance, ok := rawBalance.(map[string]any)
		if !ok {
			continue
		}
		accountIndex, indexErr := strconv.Atoi(fmt.Sprint(balance["accountIndex"]))
		if indexErr != nil || accountIndex < 0 || accountIndex >= len(keys) {
			continue
		}
		accountKey := ""
		switch key := keys[accountIndex].(type) {
		case string:
			accountKey = key
		case map[string]any:
			accountKey = fmt.Sprint(key["pubkey"])
		}
		uiAmount, ok := balance["uiTokenAmount"].(map[string]any)
		if accountKey == "" || !ok {
			continue
		}
		decimals, decimalsErr := strconv.ParseUint(fmt.Sprint(uiAmount["decimals"]), 10, 8)
		if decimalsErr != nil {
			continue
		}
		balances[accountKey] = solanaTokenAccountBalance{mint: fmt.Sprint(balance["mint"]), decimals: uint8(decimals)}
	}
	return balances
}

func ValidDirectUSDTSolanaTransfer(t SolanaTransfer, destination string, expected uint64, now int64) bool {
	if t.Signature == "" || t.InstructionIndex < 0 || t.BlockTime == nil || *t.BlockTime <= now-48*3600 || *t.BlockTime > now || t.MetaErr || t.Mint != setting.USDTSolanaMint || t.Program != setting.USDTSolanaTokenProgram || t.Destination != strings.TrimSpace(destination) || t.Amount != expected || t.Decimals != setting.USDTTONDecimals {
		return false
	}
	return setting.ValidateSolanaAddress(t.Destination) == nil
}

// PollDirectUSDTTONOnce and PollDirectUSDTSolanaOnce are intentionally
// read-only. Parsing provider responses and settlement are kept separate so a
// malformed or non-finalized provider response can never credit a balance.
func PollDirectUSDTTONOnce(ctx context.Context) error {
	if err := setting.ValidateUSDTProviderEndpoint("TON", setting.USDTTONAPIBaseURL); err != nil {
		return err
	}
	if model.DB == nil {
		return nil
	}
	// New TON invoices require a configured read-only key, but reconciliation
	// must not abandon an immutable invoice merely because that key was rotated
	// or temporarily removed. Toncenter's public endpoint can be called without
	// it; an auth-required endpoint returns an error and the watcher retries
	// after the operator restores a read-only credential. API keys are never
	// persisted in invoice snapshots.
	payments, err := model.GetPendingDirectUSDTNetworkPayments("TON")
	if err != nil {
		return err
	}
	for _, p := range payments {
		transfers, fetchErr := fetchTONTransfers(ctx, p.Address, time.Now().Unix())
		if fetchErr != nil {
			return fetchErr
		}
		for _, transfer := range transfers {
			if transfer.TraceID == "" {
				continue
			}
			proofErr := error(nil)
			transfer.FinalityProof, proofErr = fetchTONTraceFinality(ctx, transfer.TraceID, transfer.TransactionHash, transfer.LT)
			if proofErr != nil {
				return proofErr
			}
			if !ValidDirectUSDTTONTransfer(transfer, p.Address, p.ExpectedUnits, time.Now().Unix()) {
				continue
			}
			if err := settleTONTransfer(p, transfer); err != nil && !errors.Is(err, model.ErrDirectPaymentAlreadySettled) && !errors.Is(err, model.ErrDirectPaymentExpired) && !errors.Is(err, model.ErrDirectPaymentInvalid) {
				return err
			}
		}
	}
	return nil
}

func PollDirectUSDTSolanaOnce(ctx context.Context) error {
	if err := setting.ValidateUSDTProviderEndpoint("SOLANA", setting.USDTSolanaRPCURL); err != nil {
		return err
	}
	if model.DB == nil || strings.TrimSpace(setting.USDTSolanaRPCURL) == "" {
		return nil
	}
	payments, err := model.GetPendingDirectUSDTNetworkPayments("SOLANA")
	if err != nil {
		return err
	}
	for _, p := range payments {
		dest := p.Destination
		if dest == "" {
			dest = setting.USDTSolanaReceivingTokenAccount
		}
		sigs, err := fetchSolanaSignatures(ctx, dest)
		if err != nil {
			return err
		}
		for _, s := range sigs {
			if s.Err != nil || s.BlockTime == nil {
				continue
			}
			var tx any
			if err := solanaRPC(ctx, "getTransaction", []any{s.Signature, map[string]any{"encoding": "jsonParsed", "commitment": "finalized", "maxSupportedTransactionVersion": 0}}, &tx); err != nil {
				return err
			}
			blockTime := s.BlockTime
			if txMap, ok := tx.(map[string]any); ok {
				if value, ok := txMap["blockTime"].(float64); ok {
					ts := int64(value)
					blockTime = &ts
				}
			}
			for _, tr := range solanaExtractTransfers(tx, s.Signature, blockTime) {
				if !ValidDirectUSDTSolanaTransfer(tr, dest, p.ExpectedUnits, time.Now().Unix()) {
					continue
				}
				if err := model.SettleDirectUSDTTRC20Event(model.DirectUSDTTransferEvent{TradeNo: p.TradeNo, Network: "SOLANA", TxHash: s.Signature, EventIndex: strconv.Itoa(tr.InstructionIndex), EventID: directUSDTMultiChainEventID(s.Signature, tr.InstructionIndex), Contract: tr.Mint, To: tr.Destination, AmountUnits: tr.Amount, Confirmed: true, BlockTimestamp: *tr.BlockTime}); err != nil && !errors.Is(err, model.ErrDirectPaymentAlreadySettled) && !errors.Is(err, model.ErrDirectPaymentExpired) && !errors.Is(err, model.ErrDirectPaymentInvalid) {
					return err
				}
			}
		}
	}
	return nil
}

func directUSDTMultiChainEventID(signature string, instructionIndex int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(signature), instructionIndex)
}

func settleTONTransfer(p model.DirectCryptoPayment, t TONJettonTransfer) error {
	amount, _ := strconv.ParseUint(t.Amount, 10, 64)
	eventID := t.TransactionHash + ":" + t.LT
	return model.SettleDirectUSDTTRC20Event(model.DirectUSDTTransferEvent{TradeNo: p.TradeNo, Network: "TON", TxHash: t.TransactionHash, EventIndex: t.LT, EventID: eventID, Contract: t.JettonMaster, To: t.Destination, AmountUnits: amount, Confirmed: true, BlockTimestamp: t.Utime})
}
