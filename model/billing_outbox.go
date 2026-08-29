package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BillingOutboxKindLog        = "billing_log"
	BillingOutboxKindTaskRefund = "task_refund"

	billingOutboxStagePending     = "pending"
	billingOutboxStageFundingDone = "funding_done"
	billingOutboxStageTokenDone   = "token_done"
)

// BillingOutbox keeps only billing recovery work which crossed a database
// boundary. Financial adjustments stay in the main DB transaction; LOG_DB
// delivery is retried from this durable row.
type BillingOutbox struct {
	ID            int64  `json:"id" gorm:"primary_key"`
	EventID       string `json:"event_id" gorm:"type:varchar(64);uniqueIndex"`
	Kind          string `json:"kind" gorm:"type:varchar(32);index"`
	ReferenceID   string `json:"reference_id" gorm:"type:varchar(191);index"`
	Stage         string `json:"stage" gorm:"type:varchar(32);index"`
	Payload       string `json:"payload" gorm:"type:text"`
	Attempts      int    `json:"attempts"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"bigint;index"`
	LastError     string `json:"last_error" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint;index"`
}

type billingLogOutboxPayload struct {
	Log Log `json:"log"`
}

type taskRefundOutboxPayload struct {
	Reason string `json:"reason"`
}

type BillingOutboxProcessResult struct {
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
}

type ViolationFeeCommitParams struct {
	UserID          int
	TokenID         int
	TokenKey        string
	Quota           int
	SubscriptionID  int
	UseSubscription bool
	SkipTokenQuota  bool
}

func HasBillingOutboxTable() bool {
	return DB.Migrator().HasTable(&BillingOutbox{})
}

// CommitViolationFeeWithOutbox atomically debits both authoritative quota
// sources and replaces the staged zero-quota log intent with the final charge.
// It is used only after startup migration has created billing_outboxes.
func CommitViolationFeeWithOutbox(c *gin.Context, eventID string, commit ViolationFeeCommitParams, logParams RecordConsumeLogParams) error {
	if eventID == "" {
		return errors.New("billing outbox event id is required")
	}
	if commit.Quota <= 0 {
		return errors.New("violation fee quota must be positive")
	}
	if !HasBillingOutboxTable() {
		return errors.New("billing outbox table is unavailable")
	}

	log := buildConsumeLog(c, commit.UserID, logParams)
	originalRequestID := log.RequestId
	if originalRequestID != "" && originalRequestID != eventID {
		other, _ := common.StrToMap(log.Other)
		if other == nil {
			other = map[string]interface{}{}
		}
		other["original_request_id"] = originalRequestID
		log.Other = common.MapToJsonStr(other)
	}
	log.RequestId = eventID
	payload, err := common.Marshal(billingLogOutboxPayload{Log: *log})
	if err != nil {
		return err
	}

	now := common.GetTimestamp()
	event := BillingOutbox{
		EventID:       eventID,
		Kind:          BillingOutboxKindLog,
		ReferenceID:   originalRequestID,
		Stage:         billingOutboxStagePending,
		Payload:       string(payload),
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if commit.UseSubscription {
			if commit.SubscriptionID <= 0 {
				return errors.New("subscription id is missing")
			}
			var subscription UserSubscription
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", commit.SubscriptionID).
				First(&subscription).Error; err != nil {
				return err
			}
			newUsed := subscription.AmountUsed + int64(commit.Quota)
			if subscription.AmountTotal > 0 && newUsed > subscription.AmountTotal {
				return fmt.Errorf("%w, used=%d total=%d", ErrSubscriptionQuotaExceeded, newUsed, subscription.AmountTotal)
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).
				Update("amount_used", newUsed).Error; err != nil {
				return err
			}
		} else {
			result := tx.Model(&User{}).Where("id = ?", commit.UserID).
				Update("quota", gorm.Expr("quota - ?", commit.Quota))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}

		if !commit.SkipTokenQuota {
			result := tx.Model(&Token{}).Where("id = ?", commit.TokenID).Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota - ?", commit.Quota),
				"used_quota":    gorm.Expr("used_quota + ?", commit.Quota),
				"accessed_time": now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "event_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"payload":         event.Payload,
				"reference_id":    event.ReferenceID,
				"last_error":      "",
				"next_attempt_at": now,
				"updated_at":      now,
			}),
		}).Create(&event).Error
	})
	if err != nil {
		return err
	}
	invalidateUserQuotaAfterBilling(commit.UserID)
	if !commit.SkipTokenQuota {
		invalidateTokenQuotaAfterBilling(commit.TokenKey)
	}
	return nil
}

func newBillingOutboxEventID(prefix string) (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return prefix + key, nil
}

// enqueueBillingLog stores an exact copy of a failed LOG_DB write. The retry
// gets a dedicated request ID so it can detect a previous successful delivery
// after a process crash without requiring a cross-database transaction.
func enqueueBillingLog(log *Log) error {
	eventID, err := newBillingOutboxEventID("blog_")
	if err != nil {
		return err
	}
	logCopy := *log
	if logCopy.RequestId != "" {
		other, _ := common.StrToMap(logCopy.Other)
		if other == nil {
			other = map[string]interface{}{}
		}
		other["original_request_id"] = logCopy.RequestId
		logCopy.Other = common.MapToJsonStr(other)
	}
	logCopy.RequestId = eventID
	payload, err := common.Marshal(billingLogOutboxPayload{Log: logCopy})
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	return DB.Create(&BillingOutbox{
		EventID:       eventID,
		Kind:          BillingOutboxKindLog,
		ReferenceID:   logCopy.RequestId,
		Stage:         billingOutboxStagePending,
		Payload:       string(payload),
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error
}

func persistBillingLog(log *Log) error {
	if err := createLog(log); err != nil {
		if outboxErr := enqueueBillingLog(log); outboxErr != nil {
			return errors.Join(err, fmt.Errorf("enqueue billing log retry: %w", outboxErr))
		}
		return fmt.Errorf("billing log queued after LOG_DB failure: %w", err)
	}
	return nil
}

// UpsertConsumeLogOutboxIntent writes or refreshes a deterministic billing-log
// intent in the main DB. Callers create it before the financial settlement and
// replace its payload with the final settlement outcome afterwards.
func UpsertConsumeLogOutboxIntent(c *gin.Context, userID int, eventID string, params RecordConsumeLogParams) error {
	return upsertConsumeLogOutboxIntent(c, userID, eventID, params, common.GetTimestamp(), true)
}

// StageConsumeLogOutboxIntent persists a billing-log intent before the related
// financial write. The short delay prevents the periodic worker from delivering
// the provisional payload while the synchronous settlement is still running;
// the final UpsertConsumeLogOutboxIntent makes it immediately deliverable.
func StageConsumeLogOutboxIntent(c *gin.Context, userID int, eventID string, params RecordConsumeLogParams) error {
	return upsertConsumeLogOutboxIntent(c, userID, eventID, params, common.GetTimestamp()+60, false)
}

func upsertConsumeLogOutboxIntent(c *gin.Context, userID int, eventID string, params RecordConsumeLogParams, nextAttemptAt int64, deliverWithoutOutbox bool) error {
	if eventID == "" {
		return errors.New("billing outbox event id is required")
	}
	if !DB.Migrator().HasTable(&BillingOutbox{}) {
		if deliverWithoutOutbox {
			return RecordConsumeLog(c, userID, params)
		}
		return nil
	}
	log := buildConsumeLog(c, userID, params)
	originalRequestID := log.RequestId
	if originalRequestID != "" && originalRequestID != eventID {
		other, _ := common.StrToMap(log.Other)
		if other == nil {
			other = map[string]interface{}{}
		}
		other["original_request_id"] = originalRequestID
		log.Other = common.MapToJsonStr(other)
	}
	log.RequestId = eventID
	payload, err := common.Marshal(billingLogOutboxPayload{Log: *log})
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	event := BillingOutbox{
		EventID:       eventID,
		Kind:          BillingOutboxKindLog,
		ReferenceID:   originalRequestID,
		Stage:         billingOutboxStagePending,
		Payload:       string(payload),
		NextAttemptAt: nextAttemptAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"payload":         event.Payload,
			"reference_id":    event.ReferenceID,
			"last_error":      "",
			"next_attempt_at": nextAttemptAt,
			"updated_at":      now,
		}),
	}).Create(&event).Error
}

func DeliverBillingOutboxEvent(eventID string) error {
	if !DB.Migrator().HasTable(&BillingOutbox{}) {
		return nil
	}
	return ProcessBillingOutboxEvent(eventID)
}

// UpdateWithStatusAndTaskRefund atomically persists the terminal task state and
// its refund intent. A process crash can therefore leave either both or neither.
func (t *Task) UpdateWithStatusAndTaskRefund(fromStatus TaskStatus, reason string) (bool, string, error) {
	eventID := fmt.Sprintf("task_refund_%d", t.ID)
	payload, err := common.Marshal(taskRefundOutboxPayload{Reason: reason})
	if err != nil {
		return false, "", err
	}

	won := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		updated := *t
		result := tx.Model(&Task{}).Where("id = ? AND status = ?", t.ID, fromStatus).Select("*").Updates(&updated)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		won = true
		now := common.GetTimestamp()
		return tx.Create(&BillingOutbox{
			EventID:       eventID,
			Kind:          BillingOutboxKindTaskRefund,
			ReferenceID:   fmt.Sprintf("%d", t.ID),
			Stage:         billingOutboxStagePending,
			Payload:       string(payload),
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}).Error
	})
	if err != nil {
		return false, "", err
	}
	return won, eventID, nil
}

func HasPendingBillingOutbox() bool {
	var id int64
	err := DB.Model(&BillingOutbox{}).
		Where("next_attempt_at <= ?", common.GetTimestamp()).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func recordBillingOutboxFailure(id int64, processErr error) {
	now := common.GetTimestamp()
	_ = DB.Model(&BillingOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"attempts":        gorm.Expr("attempts + 1"),
		"last_error":      processErr.Error(),
		"next_attempt_at": now + 15,
		"updated_at":      now,
	}).Error
}

func ProcessBillingOutbox(ctx context.Context, limit int) BillingOutboxProcessResult {
	if limit <= 0 {
		limit = 100
	}
	var events []*BillingOutbox
	now := common.GetTimestamp()
	if err := DB.Where("next_attempt_at <= ?", now).Order("id").Limit(limit).Find(&events).Error; err != nil {
		return BillingOutboxProcessResult{Failed: 1}
	}

	result := BillingOutboxProcessResult{}
	for _, event := range events {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if err := processBillingOutboxEvent(event.EventID); err != nil {
			recordBillingOutboxFailure(event.ID, err)
			result.Failed++
			continue
		}
		result.Processed++
	}
	var pending int64
	_ = DB.Model(&BillingOutbox{}).Count(&pending).Error
	result.Pending = int(pending)
	return result
}

func ProcessBillingOutboxEvent(eventID string) error {
	err := processBillingOutboxEvent(eventID)
	if err == nil {
		return nil
	}
	var event BillingOutbox
	if findErr := DB.Where("event_id = ?", eventID).First(&event).Error; findErr == nil {
		recordBillingOutboxFailure(event.ID, err)
	}
	return err
}

func processBillingOutboxEvent(eventID string) error {
	now := common.GetTimestamp()
	claim := DB.Model(&BillingOutbox{}).
		Where("event_id = ? AND next_attempt_at <= ?", eventID, now).
		Updates(map[string]interface{}{
			"next_attempt_at": now + 60,
			"updated_at":      now,
		})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil
	}
	var event BillingOutbox
	if err := DB.Where("event_id = ?", eventID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	switch event.Kind {
	case BillingOutboxKindLog:
		return processBillingLogOutbox(&event)
	case BillingOutboxKindTaskRefund:
		return processTaskRefundOutbox(&event)
	default:
		return fmt.Errorf("unknown billing outbox kind %q", event.Kind)
	}
}

func processBillingLogOutbox(event *BillingOutbox) error {
	var payload billingLogOutboxPayload
	if err := common.UnmarshalJsonStr(event.Payload, &payload); err != nil {
		return err
	}
	return deliverBillingOutboxLog(event, &payload.Log)
}

func deliverBillingOutboxLog(event *BillingOutbox, log *Log) error {
	var count int64
	if err := LOG_DB.Model(&Log{}).Where("request_id = ?", log.RequestId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := createLog(log); err != nil {
			return err
		}
	} else {
		// A staged held-reserve intent can be delivered after its grace period.
		// If finalization later resumes, replace that preliminary view with the
		// final charge instead of keeping a stale or duplicate consume log.
		if err := LOG_DB.Model(&Log{}).Where("request_id = ?", log.RequestId).Updates(map[string]interface{}{
			"user_id":             log.UserId,
			"created_at":          log.CreatedAt,
			"type":                log.Type,
			"content":             log.Content,
			"username":            log.Username,
			"token_name":          log.TokenName,
			"model_name":          log.ModelName,
			"quota":               log.Quota,
			"prompt_tokens":       log.PromptTokens,
			"completion_tokens":   log.CompletionTokens,
			"use_time":            log.UseTime,
			"is_stream":           log.IsStream,
			"channel_id":          log.ChannelId,
			"token_id":            log.TokenId,
			"group":               log.Group,
			"ip":                  log.Ip,
			"upstream_request_id": log.UpstreamRequestId,
			"other":               log.Other,
		}).Error; err != nil {
			return err
		}
	}
	// A final payload may replace this event while LOG_DB is being written.
	// Delete only the exact payload that was delivered.
	return DB.Where("id = ? AND payload = ?", event.ID, event.Payload).Delete(&BillingOutbox{}).Error
}

func processTaskRefundOutbox(event *BillingOutbox) error {
	for {
		var current BillingOutbox
		if err := DB.Where("id = ?", event.ID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		switch current.Stage {
		case billingOutboxStagePending:
			if err := refundTaskFunding(&current); err != nil {
				return err
			}
		case billingOutboxStageFundingDone:
			if err := refundTaskToken(&current); err != nil {
				return err
			}
		case billingOutboxStageTokenDone:
			return refundTaskLog(&current)
		default:
			return fmt.Errorf("unknown task refund stage %q", current.Stage)
		}
	}
}

func refundTaskFunding(event *BillingOutbox) error {
	userID := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current BillingOutbox
		if err := tx.Where("id = ?", event.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Stage != billingOutboxStagePending {
			return nil
		}
		var task Task
		if err := tx.Where("id = ?", current.ReferenceID).First(&task).Error; err != nil {
			return err
		}
		walletRefund, err := refundTaskFundingSources(tx, &task)
		if err != nil {
			return err
		}
		if walletRefund > 0 {
			userID = task.UserId
		}
		return tx.Model(&BillingOutbox{}).Where("id = ? AND stage = ?", current.ID, billingOutboxStagePending).
			Updates(map[string]interface{}{"stage": billingOutboxStageFundingDone, "updated_at": common.GetTimestamp()}).Error
	})
	if err == nil && userID > 0 {
		invalidateUserQuotaAfterBilling(userID)
	}
	return err
}

func refundTaskToken(event *BillingOutbox) error {
	tokenKey := ""
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current BillingOutbox
		if err := tx.Where("id = ?", event.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Stage != billingOutboxStageFundingDone {
			return nil
		}
		var task Task
		if err := tx.Where("id = ?", current.ReferenceID).First(&task).Error; err != nil {
			return err
		}
		if task.PrivateData.TokenId > 0 {
			var token Token
			err := tokenReadDB(tx).Where("id = ?", task.PrivateData.TokenId).First(&token).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				tokenKey = token.Key
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
					"remain_quota":  gorm.Expr("remain_quota + ?", task.Quota),
					"used_quota":    gorm.Expr("used_quota - ?", task.Quota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&BillingOutbox{}).Where("id = ? AND stage = ?", current.ID, billingOutboxStageFundingDone).
			Updates(map[string]interface{}{"stage": billingOutboxStageTokenDone, "updated_at": common.GetTimestamp()}).Error
	})
	if err == nil && tokenKey != "" {
		invalidateTokenQuotaAfterBilling(tokenKey)
	}
	return err
}

func refundTaskLog(event *BillingOutbox) error {
	var payload taskRefundOutboxPayload
	if err := common.UnmarshalJsonStr(event.Payload, &payload); err != nil {
		return err
	}
	var task Task
	if err := DB.Where("id = ?", event.ReferenceID).First(&task).Error; err != nil {
		return err
	}
	username, _ := GetUsernameById(task.UserId, false)
	tokenName := ""
	if task.PrivateData.TokenId > 0 {
		if token, err := GetTokenById(task.PrivateData.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	other := map[string]interface{}{
		"task_id": task.TaskID,
		"reason":  payload.Reason,
	}
	if task.PrivateData.BillingSource != "" {
		other["billing_source"] = task.PrivateData.BillingSource
	}
	if task.PrivateData.BillingOverageSource != "" {
		other["billing_overage_source"] = task.PrivateData.BillingOverageSource
		other["billing_overage_quota"] = task.PrivateData.BillingOverageQuota
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		for key, ratio := range bc.OtherRatios {
			other[key] = ratio
		}
	}
	log := &Log{
		UserId:    task.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeRefund,
		Content:   "",
		TokenName: tokenName,
		ModelName: task.Properties.OriginModelName,
		Quota:     task.Quota,
		ChannelId: task.ChannelId,
		TokenId:   task.PrivateData.TokenId,
		Group:     ratio_setting.PricingGroupKeyOrDefault(task.Group),
		RequestId: event.EventID,
		Other:     common.MapToJsonStr(other),
	}
	return deliverBillingOutboxLog(event, log)
}
