package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type systemUpdateApplyRequest struct {
	Version string `json:"version"`
}

func CheckSystemUpdate(c *gin.Context) {
	result, err := service.CheckSystemUpdate(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func ApplySystemUpdate(c *gin.Context) {
	request := systemUpdateApplyRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request body",
		})
		return
	}

	task, _, err := service.StartSystemUpdateTask(request.Version)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    task.ToResponse(),
	})
}

func GetSystemUpdateJob(c *gin.Context) {
	jobID := c.Param("job_id")
	status, err := service.GetSystemUpdaterJobStatus(c.Request.Context(), jobID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    status,
	})
}
