package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type supplierPricingMutation struct {
	ExpectedConfigRevision *int64                      `json:"expected_config_revision"`
	ExpectedRevision       *int64                      `json:"expected_revision"`
	Tariffs                []types.SupplierModelTariff `json:"tariffs"`
}

type supplierPricingSyncResult struct {
	Revision            int64 `json:"revision"`
	PendingConfirmation bool  `json:"pending_confirmation"`
	Models              int   `json:"models"`
}

func supplierPricingFailure(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, model.ErrSupplierPricingConflict) {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}

func supplierPricingProfileID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		supplierPricingFailure(c, errors.New("valid pricing profile id required"))
		return 0, false
	}
	return id, true
}

func readSupplierPricingMutation(c *gin.Context) (supplierPricingMutation, bool) {
	request := supplierPricingMutation{}
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, maxSupplierPricingBytes), &request); err != nil {
		supplierPricingFailure(c, errors.New("invalid supplier pricing request"))
		return request, false
	}
	if request.ExpectedConfigRevision == nil || request.ExpectedRevision == nil || *request.ExpectedConfigRevision < 0 || *request.ExpectedRevision < 0 {
		supplierPricingFailure(c, errors.New("expected_config_revision and expected_revision are required"))
		return request, false
	}
	return request, true
}

func GetSupplierPricingTariffs(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPricingProfileID(c)
	if !ok {
		return
	}
	profile, err := model.GetSupplierPricingProfile(id)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	tariffs, revision, err := model.ListSupplierProfileTariffs(id)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"tariffs": tariffs, "revision": revision, "config_revision": profile.ConfigRevision}})
}

func PublishSupplierPricingTariffs(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPricingProfileID(c)
	if !ok {
		return
	}
	request, ok := readSupplierPricingMutation(c)
	if !ok {
		return
	}
	if request.Tariffs == nil {
		supplierPricingFailure(c, errors.New("tariffs must be a complete array; use [] to remove every model"))
		return
	}
	revision, err := model.PublishSupplierProfileTariffs(id, *request.ExpectedConfigRevision, *request.ExpectedRevision, request.Tariffs)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	recordManageAudit(c, "supplier.pricing.publish", map[string]any{"profile_id": id, "revision": revision, "models": len(request.Tariffs)})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"revision": revision}})
}

func runSupplierPricingProfileSync(ctx context.Context, profile model.SupplierPricingProfile, expectedRevision int64) (supplierPricingSyncResult, error) {
	result := supplierPricingSyncResult{Revision: expectedRevision}
	supplier, err := model.GetSupplier(profile.SupplierID)
	if err != nil {
		return result, err
	}
	now := time.Now().Unix()
	update := model.DB.Model(&model.SupplierPricingProfile{}).Where("id = ? AND config_revision = ?", profile.ID, profile.ConfigRevision).Update("last_attempt_at", now)
	if update.Error != nil {
		return result, update.Error
	}
	tariffs, err := fetchSupplierPricing(ctx, supplier, profile)
	if err == nil {
		result.Revision, result.PendingConfirmation, err = model.ConfirmSupplierProfileTariffs(profile.ID, profile.ConfigRevision, expectedRevision, tariffs)
		result.Models = len(tariffs)
	}
	status := map[string]any{"last_error": "", "last_success_at": now}
	if err != nil {
		status = map[string]any{"last_error": err.Error()}
	}
	statusErr := model.DB.Model(&model.SupplierPricingProfile{}).Where("id = ? AND config_revision = ?", profile.ID, profile.ConfigRevision).Updates(status).Error
	if err == nil {
		err = statusErr
	}
	return result, err
}

func SyncSupplierPricingProfile(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPricingProfileID(c)
	if !ok {
		return
	}
	request, ok := readSupplierPricingMutation(c)
	if !ok {
		return
	}
	profile, err := model.GetSupplierPricingProfile(id)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	_, revision, err := model.ListSupplierProfileTariffs(id)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	if profile.ConfigRevision != *request.ExpectedConfigRevision || revision != *request.ExpectedRevision {
		supplierPricingFailure(c, model.ErrSupplierPricingConflict)
		return
	}
	result, err := runSupplierPricingProfileSync(c.Request.Context(), profile, revision)
	if err != nil {
		supplierPricingFailure(c, err)
		return
	}
	recordManageAudit(c, "supplier.pricing.sync", map[string]any{"profile_id": id, "revision": result.Revision, "pending_confirmation": result.PendingConfirmation})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type supplierPricingSyncHandler struct{}

func (supplierPricingSyncHandler) Type() string            { return "supplier_pricing_sync" }
func (supplierPricingSyncHandler) Interval() time.Duration { return time.Minute }
func (supplierPricingSyncHandler) NewPayload() any         { return nil }
func (supplierPricingSyncHandler) Enabled() bool {
	profiles, err := model.ListSupplierPricingProfiles(0)
	if err != nil {
		return false
	}
	for _, profile := range profiles {
		if !profile.Enabled || profile.IntervalSeconds <= 0 {
			continue
		}
		supplier, err := model.GetSupplier(profile.SupplierID)
		if err == nil && supplier.Enabled && supplier.Adapter != "manual" {
			return true
		}
	}
	return false
}

func (supplierPricingSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	profiles, err := model.ListSupplierPricingProfiles(0)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	checked, failed := 0, 0
	for _, profile := range profiles {
		if err := ctx.Err(); err != nil {
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
			return
		}
		if !profile.Enabled || profile.IntervalSeconds <= 0 || time.Now().Unix()-profile.LastAttemptAt < int64(profile.IntervalSeconds) {
			continue
		}
		supplier, lookupErr := model.GetSupplier(profile.SupplierID)
		if lookupErr == nil && (!supplier.Enabled || supplier.Adapter == "manual") {
			continue
		}
		_, revision, err := model.ListSupplierProfileTariffs(profile.ID)
		if err == nil {
			_, err = runSupplierPricingProfileSync(ctx, profile, revision)
		}
		checked++
		if err != nil {
			failed++
		}
	}
	status := model.SystemTaskStatusSucceeded
	if failed > 0 {
		status = model.SystemTaskStatusFailed
		err = fmt.Errorf("supplier pricing sync failed for %d of %d profiles", failed, checked)
	}
	finishSystemTaskHandler(task, runnerID, status, map[string]int{"checked_profiles": checked, "failed_profiles": failed}, err)
}

// StartSupplierPricingSyncTask registers a DB-leased job; the system runner owns
// scheduling and cancellation, so no independent background goroutine is needed.
func StartSupplierPricingSyncTask() {
	service.RegisterSystemTaskHandler(supplierPricingSyncHandler{})
}
