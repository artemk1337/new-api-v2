package controller

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// GetPlatformTags also includes tags written by older application versions.
func GetPlatformTags(c *gin.Context) {
	tags, err := model.GetPlatformTags()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelTags, err := model.GetAllChannelTags()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	seen := make(map[string]struct{}, len(tags)+len(channelTags))
	add := func(name string) {
		if strings.TrimSpace(name) != "" {
			seen[name] = struct{}{}
		}
	}
	for _, name := range tags {
		add(name)
	}
	for _, name := range channelTags {
		add(name)
	}
	for _, group := range ratio_setting.GetPricingGroupsCopy() {
		add(group.Tag)
	}
	tags = tags[:0]
	for name := range seen {
		tags = append(tags, name)
	}
	sort.Strings(tags)
	common.ApiSuccess(c, tags)
}

func CreatePlatformTag(c *gin.Context) {
	var request struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || utf8.RuneCountInString(name) > 191 {
		common.ApiErrorMsg(c, "Tag must contain between 1 and 191 characters")
		return
	}
	name, err := model.CreatePlatformTag(name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, name)
}
