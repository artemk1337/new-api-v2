package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetDockerImageStats(c *gin.Context) {
	stats, err := service.GetDockerImageStats(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func ClearDockerImages(c *gin.Context) {
	result, err := service.ClearDockerImages(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetBackupStats(c *gin.Context) {
	stats, err := service.GetBackupStats(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func ClearOldBackups(c *gin.Context) {
	result, err := service.ClearOldBackups(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
