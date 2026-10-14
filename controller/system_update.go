package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type systemUpdateApplyRequest struct {
	Version string `json:"version"`
}

func CheckSystemUpdate(c *gin.Context) {
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "5"))
	if pageErr != nil || sizeErr != nil || page < 1 || pageSize < 1 || pageSize > 10 {
		common.ApiError(c, errors.New("page must be positive and page_size must be between 1 and 10"))
		return
	}
	result, err := service.CheckSystemUpdatePage(c.Request.Context(), page, pageSize)
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

	task, _, err := service.StartSystemUpdateTask(c.Request.Context(), request.Version)
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
