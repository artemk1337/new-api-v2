package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminTokenResponse struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Status       int    `json:"status"`
	Group        string `json:"group"`
	CreatedTime  int64  `json:"created_time"`
	AccessedTime int64  `json:"accessed_time"`
}

func tokenAdminResponse(token *model.Token) adminTokenResponse {
	return adminTokenResponse{token.Id, token.Name, token.Status, token.Group, token.CreatedTime, token.AccessedTime}
}

func manageableTokenUser(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return 0, false
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return 0, false
	}
	role := c.GetInt("role")
	if role < common.RoleAdminUser || !canManageTargetRole(role, user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return 0, false
	}
	return id, true
}

func GetUserTokensByAdmin(c *gin.Context) {
	userId, ok := manageableTokenUser(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, err := model.CountUserTokens(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]adminTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, tokenAdminResponse(token))
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func SetUserTokenStatusByAdmin(c *gin.Context) {
	userId, ok := manageableTokenUser(c)
	if !ok {
		return
	}
	tokenId, err := strconv.Atoi(c.Param("token_id"))
	var request struct {
		Status int `json:"status"`
	}
	if err != nil || tokenId <= 0 || c.ShouldBindJSON(&request) != nil ||
		(request.Status != common.TokenStatusEnabled && request.Status != common.TokenStatusDisabled) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status := request.Status
	if status == common.TokenStatusDisabled {
		status = common.TokenStatusAdminDisabled
	} else if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
		status = common.TokenStatusExpired
	} else if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		status = common.TokenStatusExhausted
	}
	if err := model.SetTokenAdminStatus(token, status); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tokenAdminResponse(token))
}
