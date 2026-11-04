package controller

import (
	"math"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func rejectUserProxySupplierFilter(c *gin.Context) bool {
	if _, exists := c.Request.URL.Query()["proxy_supplier_id"]; !exists {
		return false
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient permissions"})
	return true
}

func adminLogFilter(c *gin.Context) (model.LogFilter, bool) {
	filter := model.LogFilter{
		ModelName: c.Query("model_name"), Username: c.Query("username"), TokenName: c.Query("token_name"),
		Group: c.Query("group"), RequestID: c.Query("request_id"), UpstreamRequestID: c.Query("upstream_request_id"),
	}
	filter.Type, _ = strconv.Atoi(c.Query("type"))
	filter.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	filter.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	filter.Channel, _ = strconv.Atoi(c.Query("channel"))
	if _, exists := c.Request.URL.Query()["proxy_supplier_id"]; exists {
		if c.GetInt("role") < common.RoleAdminUser {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient permissions"})
			return filter, false
		}
		id, err := strconv.Atoi(c.Query("proxy_supplier_id"))
		if err != nil || id < 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid proxy supplier ID"})
			return filter, false
		}
		filter.ProxySupplierID = &id
	}
	return filter, true
}

func logSuggestionQuery(c *gin.Context) (model.LogFilter, string, int, int, bool) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient permissions"})
		return model.LogFilter{}, "", 0, 0, false
	}
	filter, ok := adminLogFilter(c)
	if !ok {
		return filter, "", 0, 0, false
	}
	q := c.Query("q")
	offset, limit := 0, 50
	var err error
	if cursor := c.Query("cursor"); cursor != "" {
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 || offset > math.MaxInt-101 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid cursor"})
			return filter, q, 0, 0, false
		}
	}
	if value, exists := c.Request.URL.Query()["limit"]; exists {
		limit, err = strconv.Atoi(value[0])
		if err != nil || limit < 1 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid limit"})
			return filter, q, 0, 0, false
		}
		limit = min(limit, 100)
	}
	if len(q) > 256 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "query is too long"})
		return filter, q, 0, 0, false
	}
	return filter, q, offset, limit, true
}

func GetLogSuggestions(c *gin.Context) {
	filter, q, offset, limit, ok := logSuggestionQuery(c)
	if !ok {
		return
	}
	field := c.Query("field")
	if field != "model_name" && field != "group" && field != "token_name" && field != "username" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid log suggestion field"})
		return
	}
	page, err := model.GetLogSuggestions(c.Request.Context(), filter, field, q, offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func GetLogSupplierChoices(c *gin.Context) {
	filter, q, offset, limit, ok := logSuggestionQuery(c)
	if !ok {
		return
	}
	page, err := model.GetLogSupplierChoices(c.Request.Context(), filter, q, offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}
