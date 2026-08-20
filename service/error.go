package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"
)

func MidjourneyErrorWrapper(code int, desc string) *dto.MidjourneyResponse {
	return &dto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *dto.MidjourneyResponseWithStatusCode {
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		newApiErr.SetFinancialOutcome(types.AttemptFinancialOutcomeAmbiguous)
		return
	}
	value := gjson.ParseBytes(responseBody)
	upstreamUsage, upstreamCost, upstreamCostSet := extractAuthoritativeBillingPayload(value)
	financialOutcome := ClassifyUpstreamErrorResponse(normalizedUpstreamStatusCode(resp.StatusCode, responseBody), responseBody)
	defer func() {
		normalizeUpstreamSaturationStatus(newApiErr, resp.StatusCode, responseBody)
		if financialOutcome != types.AttemptFinancialOutcomeUnknown {
			newApiErr.SetFinancialOutcome(financialOutcome)
		}
		newApiErr.SetUpstreamBillingEvidence(upstreamUsage, upstreamCost, upstreamCostSet)
	}()
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse
	responseBodyText := string(responseBody)
	responseBodyPreview := common.LocalLogPreview(responseBodyText)
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, responseBodyText)
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, responseBodyText)
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, responseBodyPreview))
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
	}
	newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return
}

// normalizeUpstreamSaturationStatus maps a gateway response to the client
// rate-limit status only when the body explicitly identifies upstream
// saturation. Other 502 responses retain their original status.
func normalizeUpstreamSaturationStatus(err *types.NewAPIError, statusCode int, body []byte) {
	if err == nil {
		return
	}
	err.StatusCode = normalizedUpstreamStatusCode(statusCode, body)
}

func normalizedUpstreamStatusCode(statusCode int, body []byte) int {
	if statusCode == http.StatusBadGateway && isUpstreamSaturatedBody(body) {
		return http.StatusTooManyRequests
	}
	return statusCode
}

func isUpstreamSaturatedBody(body []byte) bool {
	if gjson.ValidBytes(body) {
		return isUpstreamSaturatedResponse(gjson.ParseBytes(body))
	}
	return isUpstreamSaturatedMessage(string(body))
}

// ClassifyUpstreamErrorResponse classifies a complete, structured terminal
// upstream response independently from retry policy and HTTP status. Invalid,
// empty, or structurally unknown bodies stay unknown: after dispatch the caller
// must treat them as financially ambiguous.
func ClassifyUpstreamErrorResponse(statusCode int, body []byte) types.AttemptFinancialOutcome {
	if !gjson.ValidBytes(body) {
		return types.AttemptFinancialOutcomeUnknown
	}
	value := gjson.ParseBytes(body)
	if responseHasBillingEvidence(value) {
		return types.AttemptFinancialOutcomeBillable
	}
	if responseExplicitlyGuaranteesNoBilling(value) {
		return types.AttemptFinancialOutcomeNonBillable
	}
	// A saturated upstream rejects the request before processing it. Keep this
	// narrow to the messages used by the relay's saturation normalization; a
	// generic 429 remains financially unknown and must not be retried after
	// dispatch.
	if statusCode == http.StatusTooManyRequests && isUpstreamSaturatedResponse(value) {
		return types.AttemptFinancialOutcomeNonBillable
	}
	return types.AttemptFinancialOutcomeUnknown
}

func isUpstreamSaturatedResponse(value gjson.Result) bool {
	if value.Get("error.message").Exists() && isUpstreamSaturatedMessage(value.Get("error.message").String()) {
		return true
	}
	return isUpstreamSaturatedMessage(value.Get("message").String())
}

func isUpstreamSaturatedMessage(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "the upstream load for the current group is saturated") ||
		strings.Contains(message, "upstream overloaded") ||
		strings.Contains(message, "upstream is overloaded") ||
		strings.Contains(message, "当前分组上游负载已饱和") ||
		(strings.Contains(message, "当前模型") && strings.Contains(message, "上游已饱和")) ||
		strings.Contains(message, "所有可用凭据均已达到并发上限")
}

func responseExplicitlyGuaranteesNoBilling(value gjson.Result) bool {
	for _, container := range authoritativeBillingContainers(value) {
		cost := container.Get("cost")
		if cost.Exists() && cost.Type == gjson.Number && cost.Float() == 0 {
			return true
		}
		for _, field := range []string{"billed", "billable", "charged"} {
			marker := container.Get(field)
			if marker.Exists() && marker.Type == gjson.False {
				return true
			}
		}
	}
	return false
}

func responseHasBillingEvidence(value gjson.Result) bool {
	for _, container := range authoritativeBillingContainers(value) {
		usage := container.Get("usage")
		if hasNonEmptyJSONValue(usage) {
			return true
		}
		cost := container.Get("cost")
		if cost.Exists() && cost.Type != gjson.Null &&
			(cost.Type != gjson.Number || cost.Float() != 0) {
			return true
		}
		for _, field := range []string{"billed", "billable", "charged"} {
			marker := container.Get(field)
			if marker.Exists() && marker.Type == gjson.True {
				return true
			}
		}
	}

	evidenceContainers := []gjson.Result{value}
	if directError := value.Get("error"); directError.IsObject() {
		evidenceContainers = append(evidenceContainers, directError)
	}
	for _, container := range evidenceContainers {
		for _, field := range []string{"task_id", "taskid", "job_id", "operation_id", "prediction_id"} {
			if identifierIsPresent(container.Get(field)) {
				return true
			}
		}
		if identifierIsPresent(container.Get("operation.name")) {
			return true
		}
		for _, field := range []string{"result", "output", "choices"} {
			if hasNonEmptyJSONValue(container.Get(field)) {
				return true
			}
		}
	}
	return false
}

func authoritativeBillingContainers(value gjson.Result) []gjson.Result {
	if !value.IsObject() {
		return nil
	}
	containers := []gjson.Result{value}
	for _, path := range []string{"error", "billing", "error.billing"} {
		child := value.Get(path)
		if child.IsObject() {
			containers = append(containers, child)
		}
	}
	return containers
}

func extractAuthoritativeBillingPayload(value gjson.Result) (json.RawMessage, float64, bool) {
	var usage json.RawMessage
	for _, path := range []string{"usage", "error.usage"} {
		child := value.Get(path)
		if child.IsObject() && hasNonEmptyJSONValue(child) {
			usage = json.RawMessage(child.Raw)
			break
		}
	}

	for _, path := range []string{
		"cost",
		"usage.cost",
		"error.cost",
		"error.usage.cost",
		"billing.cost",
		"error.billing.cost",
	} {
		child := value.Get(path)
		if child.Exists() && child.Type == gjson.Number {
			return usage, child.Float(), true
		}
	}
	return usage, 0, false
}

func hasNonEmptyJSONValue(value gjson.Result) bool {
	if !value.Exists() || value.Type == gjson.Null {
		return false
	}
	switch value.Type {
	case gjson.String:
		return strings.TrimSpace(value.String()) != ""
	case gjson.JSON:
		raw := strings.TrimSpace(value.Raw)
		return raw != "" && raw != "{}" && raw != "[]"
	default:
		return false
	}
}

func identifierIsPresent(value gjson.Result) bool {
	if !value.Exists() || value.Type == gjson.Null {
		return false
	}
	switch value.Type {
	case gjson.String:
		return strings.TrimSpace(value.String()) != ""
	case gjson.Number:
		return true
	default:
		return false
	}
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NewAPIError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NewAPIError) *dto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &dto.TaskError{
		Code:             string(apiErr.GetErrorCode()),
		Message:          apiErr.Err.Error(),
		StatusCode:       apiErr.StatusCode,
		LocalError:       types.IsSkipRetryError(apiErr),
		Error:            apiErr.Err,
		FinancialOutcome: apiErr.GetFinancialOutcome(),
	}
}
