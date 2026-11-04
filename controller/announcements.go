package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAnnouncements(c *gin.Context) {
	settings, err := model.GetAnnouncementSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

func SaveAnnouncements(c *gin.Context) {
	var request model.AnnouncementSettings
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid announcement settings"})
		return
	}
	settings, err := model.SaveAnnouncementSettings(request)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrAnnouncementRevisionConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	// The durable intent is already committed. The scheduler also picks it up
	// if an immediate wakeup fails, so a saved announcement remains successful.
	if err := service.EnqueueAnnouncementTelegramDelivery(); err != nil {
		common.SysError("Failed to wake announcement delivery worker")
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

func RetryAnnouncementTelegram(c *gin.Context) {
	var request struct {
		ConfirmUnknown bool `json:"confirmUnknown"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid retry request"})
		return
	}
	if err := model.RetryAnnouncementTelegramDelivery(c.Param("id"), request.ConfirmUnknown); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := service.EnqueueAnnouncementTelegramDelivery(); err != nil {
		common.SysError("Failed to wake announcement delivery worker")
	}
	GetAnnouncements(c)
}
