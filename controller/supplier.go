package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func requireSupplierRoot(c *gin.Context) bool {
	if c.GetInt("role") < common.RoleRootUser {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient permissions"})
		return false
	}
	return true
}

func supplierPathID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid supplier or profile ID"))
		return 0, false
	}
	return id, true
}

func channelSupplierFilter(c *gin.Context) (int, bool) {
	value, exists := c.GetQuery("supplier_id")
	if !exists {
		return -1, true
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 0 {
		common.ApiError(c, errors.New("invalid supplier filter"))
		return 0, false
	}
	return id, true
}

func GetSuppliers(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	data, err := model.ListSuppliers()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func CreateSupplier(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	var supplier model.Supplier
	if err := common.DecodeJson(c.Request.Body, &supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreateSupplier(&supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.create", map[string]any{"id": supplier.ID})
	common.ApiSuccess(c, supplier)
}

func UpdateSupplier(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	var supplier model.Supplier
	if err := common.DecodeJson(c.Request.Body, &supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	supplier.ID = id
	if err := model.UpdateSupplier(&supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.update", map[string]any{"id": id})
	common.ApiSuccess(c, supplier)
}

func DeleteSupplier(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	if err := model.DeleteSupplier(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.delete", map[string]any{"id": id})
	common.ApiSuccess(c, nil)
}

func GetSupplierProfiles(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	if _, err := model.GetSupplier(id); err != nil {
		common.ApiError(c, err)
		return
	}
	profiles, err := model.ListSupplierPricingProfiles(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, profiles)
}

func CreateSupplierProfile(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	var profile model.SupplierPricingProfile
	if err := common.DecodeJson(c.Request.Body, &profile); err != nil {
		common.ApiError(c, err)
		return
	}
	if profile.SupplierID != 0 && profile.SupplierID != id {
		common.ApiError(c, errors.New("supplier profile parent does not match URL"))
		return
	}
	profile.SupplierID = id
	if err := model.CreateSupplierPricingProfile(&profile); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.profile.create", map[string]any{"id": profile.ID, "supplier_id": id})
	common.ApiSuccess(c, profile)
}

func UpdateSupplierProfile(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	var profile model.SupplierPricingProfile
	if err := common.DecodeJson(c.Request.Body, &profile); err != nil {
		common.ApiError(c, err)
		return
	}
	profile.ID = id
	if err := model.UpdateSupplierPricingProfile(&profile); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.profile.update", map[string]any{"id": id, "supplier_id": profile.SupplierID})
	common.ApiSuccess(c, profile)
}

func DeleteSupplierProfile(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	id, ok := supplierPathID(c)
	if !ok {
		return
	}
	if err := model.DeleteSupplierPricingProfile(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "supplier.profile.delete", map[string]any{"id": id})
	common.ApiSuccess(c, nil)
}

func GetSupplierGroupBindings(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	bindings, err := model.ListPricingGroupSupplierBindings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, bindings)
}

func GetSupplierMigrationPreview(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	preview, err := model.GetSupplierMigrationPreview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func UpdateSupplierGroupBindings(c *gin.Context) {
	if !requireSupplierRoot(c) {
		return
	}
	var request struct {
		Bindings []model.SupplierGroupBindingPatch   `json:"bindings"`
		Channels []model.SupplierChannelBindingPatch `json:"channels"`
	}
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := common.Unmarshal(rawBody, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	var fields map[string][]map[string]any
	if err := common.Unmarshal(rawBody, &fields); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, key := range []string{"bindings", "channels"} {
		for _, patch := range fields[key] {
			_, hasSupplier := patch["supplier_id"]
			_, hasProfile := patch["profile_id"]
			if !hasSupplier || !hasProfile {
				common.ApiError(c, errors.New("supplier_id and profile_id are required; use explicit null values to clear a binding"))
				return
			}
		}
	}
	if len(request.Bindings) > 10000 || len(request.Channels) > 10000 {
		common.ApiError(c, errors.New("too many supplier binding changes"))
		return
	}
	if err := model.ApplySupplierBindings(request.Bindings, request.Channels); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "supplier.bindings.update", map[string]any{"groups": len(request.Bindings), "channels": len(request.Channels)})
	common.ApiSuccess(c, nil)
}

type supplierProfileChoice struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type supplierChoice struct {
	ID       int                     `json:"id"`
	Name     string                  `json:"name"`
	Enabled  bool                    `json:"enabled"`
	Profiles []supplierProfileChoice `json:"profiles"`
}

func GetSupplierChoices(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser || !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelSensitiveWrite) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient permissions"})
		return
	}
	suppliers, err := model.ListSuppliers()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	profiles, err := model.ListSupplierPricingProfiles(0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	profilesBySupplier := make(map[int][]supplierProfileChoice)
	for _, profile := range profiles {
		profilesBySupplier[profile.SupplierID] = append(profilesBySupplier[profile.SupplierID], supplierProfileChoice{ID: profile.ID, Name: profile.Name, Enabled: profile.Enabled})
	}
	choices := make([]supplierChoice, 0, len(suppliers))
	for _, supplier := range suppliers {
		profiles := profilesBySupplier[supplier.ID]
		if profiles == nil {
			profiles = make([]supplierProfileChoice, 0)
		}
		choices = append(choices, supplierChoice{ID: supplier.ID, Name: supplier.Name, Enabled: supplier.Enabled, Profiles: profiles})
	}
	common.ApiSuccess(c, choices)
}
