package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupplierProcurementHandlersRejectOrdinaryUsers(t *testing.T) {
	for _, handler := range []gin.HandlerFunc{GetSuppliers, CreateSupplier, UpdateSupplier, DeleteSupplier, GetSupplierProfiles, CreateSupplierProfile, UpdateSupplierProfile, DeleteSupplierProfile, GetSupplierGroupBindings, UpdateSupplierGroupBindings, GetSupplierMigrationPreview, GetSupplierChoices} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("role", common.RoleCommonUser)
		ctx.Set("id", 7)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		handler(ctx)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "supplier_id")
	}
}

func TestSupplierBindingClearRequiresExplicitNullPair(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/suppliers/group-bindings", bytes.NewBufferString(`{"bindings":[{"group_id":2}]}`))
	UpdateSupplierGroupBindings(ctx)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, recorder.Body.String(), "explicit null")
}

func TestSupplierChoicesContainOnlyAdministrativeDisplayFields(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Supplier{}, &model.SupplierPricingProfile{}))
	supplier := model.Supplier{Code: "private-code", Name: "Supplier", Adapter: "manual", Enabled: true}
	require.NoError(t, db.Create(&supplier).Error)
	require.NoError(t, db.Create(&model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "private-profile", Name: "Profile", Enabled: true, USDPerCredit: 0.006, BaseURL: "https://private-provider.example", LastError: "private-sync-error"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/suppliers/choices", nil)
	GetSupplierChoices(ctx)
	var response struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.Len(t, response.Data[0], 4)
	assert.Equal(t, "Supplier", response.Data[0]["name"])
	assert.NotContains(t, recorder.Body.String(), "private-")
	assert.NotContains(t, recorder.Body.String(), "usd_per_credit")
}

func TestTagGroupEditRequiresSensitivePermissionForSupplierChannels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}))
	originalMaster := common.IsMasterNode
	common.IsMasterNode = true
	err := authz.Init(db)
	common.IsMasterNode = originalMaster
	require.NoError(t, err)
	require.True(t, authz.Can(7, common.RoleAdminUser, authz.ChannelWrite))
	require.False(t, authz.Can(7, common.RoleAdminUser, authz.ChannelSensitiveWrite))
	tag, supplierID, profileID := "supplier-tag", 1, 1
	channel := model.Channel{Name: "bound", Key: "secret", Tag: &tag, Models: "model", Group: "2,3", SupplierID: &supplierID, SupplierProfileID: &profileID}
	require.NoError(t, db.Create(&channel).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 7)
	ctx.Set("role", common.RoleAdminUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", bytes.NewBufferString(`{"tag":"supplier-tag","groups":"2"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	EditTagChannels(ctx)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "2,3", reloaded.Group, "denied batch edit must preserve every membership")
}

func TestChannelSupplierFieldsAreSensitiveOnlyWhenChanged(t *testing.T) {
	supplierID, profileID, otherID := 1, 2, 3
	origin := model.Channel{SupplierID: &supplierID, SupplierProfileID: &profileID, Group: "2,3"}
	updated := PatchChannel{Channel: origin}
	assert.False(t, channelHasSensitiveChanges(&updated, &origin, map[string]any{"supplier_id": 1, "supplier_profile_id": 2}))
	updated.SupplierProfileID = &otherID
	assert.True(t, channelHasSensitiveChanges(&updated, &origin, map[string]any{"supplier_profile_id": 3}))
	updated = PatchChannel{Channel: origin}
	updated.Group = "2"
	assert.True(t, channelHasSensitiveChanges(&updated, &origin, map[string]any{"group": "2"}))
	updated = PatchChannel{Channel: origin}
	updated.SupplierID, updated.SupplierProfileID = nil, nil
	assert.True(t, channelHasSensitiveChanges(&updated, &origin, map[string]any{"supplier_id": nil, "supplier_profile_id": nil}))
}
