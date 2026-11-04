package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetOfficialPrices(c *gin.Context) {
	prices, err := model.ListOfficialModelPrices()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, prices)
}

func UpdateOfficialPrices(c *gin.Context) {
	var request struct {
		Prices []model.OfficialModelPrice `json:"prices"`
	}
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SaveOfficialModelPrices(request.Prices); err != nil {
		common.ApiError(c, err)
		return
	}
	GetOfficialPrices(c)
}

func DeleteOfficialPrice(c *gin.Context) {
	var request struct {
		ModelName string `json:"model_name"`
	}
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteOfficialModelPrice(request.ModelName); err != nil {
		common.ApiError(c, err)
		return
	}
	GetOfficialPrices(c)
}
