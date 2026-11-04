package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func KeySessionLogin(c *gin.Context) {
	var request struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	key := strings.TrimPrefix(strings.TrimSpace(request.Key), "sk-")
	token, err := model.GetTokenByKey(key, true)
	if err != nil || !middleware.KeySessionTokenAllowed(c, token) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid or unavailable API key"})
		return
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("key_session_id", token.Id)
	session.Set("key_session_fingerprint", middleware.KeySessionFingerprint(token.Key))
	if err := session.Save(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func KeySessionInfo(c *gin.Context) {
	token := c.MustGet("key_session_token").(*model.Token)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"name": token.Name, "used_quota": token.UsedQuota, "remain_quota": token.RemainQuota,
		"unlimited_quota": token.UnlimitedQuota, "expired_time": token.ExpiredTime,
	}})
}

func KeySessionLogs(c *gin.Context) {
	if rejectUserProxySupplierFilter(c) {
		return
	}
	token := c.MustGet("key_session_token").(*model.Token)
	logs, err := model.GetLogByTokenId(token.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		items = append(items, gin.H{
			"id": log.Id, "created_at": log.CreatedAt, "type": log.Type,
			"model_name": log.ModelName, "quota": log.Quota,
			"prompt_tokens": log.PromptTokens, "completion_tokens": log.CompletionTokens,
			"use_time": log.UseTime, "is_stream": log.IsStream,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": items})
}
