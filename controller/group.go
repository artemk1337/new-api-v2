package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type pricingGroupResponse struct {
	Id           int                     `json:"id"`
	Name         string                  `json:"name"`
	Ratio        float64                 `json:"ratio"`
	Selectable   bool                    `json:"selectable"`
	Description  string                  `json:"description,omitempty"`
	ChannelStats model.ChannelGroupStats `json:"channel_stats"`
}

func GetGroups(c *gin.Context) {
	groups := ratio_setting.GetPricingGroupsCopy()
	channelStats, err := model.GetChannelGroupStats()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data := make([]pricingGroupResponse, 0, len(groups))
	for _, group := range groups {
		stats := channelStats[ratio_setting.PricingGroupKey(group.Name)]
		data = append(data, pricingGroupResponse{
			Id:           group.Id,
			Name:         group.Name,
			Ratio:        group.Ratio,
			Selectable:   group.Selectable,
			Description:  group.Description,
			ChannelStats: stats,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userId := c.GetInt("id")
	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	autoGroups := service.GetUserAutoGroup(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// Usable groups are derived from selectable pricing groups.
		if desc, ok := userUsableGroups[groupName]; ok {
			displayName := ratio_setting.PricingGroupNameByKey(groupName)
			if displayName == "" {
				displayName = groupName
			}
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
				"id":    groupName,
				"name":  displayName,
			}
		}
	}
	if len(autoGroups) > 0 {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  "自动分组",
			"id":    "auto",
			"name":  "auto",
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "",
		"data":        usableGroups,
		"auto_groups": autoGroups,
	})
}
