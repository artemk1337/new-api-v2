package relay

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func parseSupplierTaskResponse(c *gin.Context, parse func() (string, []byte, *dto.TaskError)) (string, []byte, *dto.TaskError) {
	originalWriter := c.Writer
	c.Writer = &supplierTaskResponseWriter{ResponseWriter: originalWriter, header: make(http.Header), status: http.StatusOK, size: -1}
	defer func() { c.Writer = originalWriter }()
	return parse()
}

// Supplier task adaptors still parse and retain their native response, but
// native payloads and headers must never become a public procurement API.
type supplierTaskResponseWriter struct {
	gin.ResponseWriter
	header http.Header
	status int
	size   int
}

func (w *supplierTaskResponseWriter) Header() http.Header    { return w.header }
func (w *supplierTaskResponseWriter) WriteHeader(status int) { w.status = status }
func (w *supplierTaskResponseWriter) WriteHeaderNow() {
	if w.size < 0 {
		w.size = 0
	}
}
func (w *supplierTaskResponseWriter) Write(body []byte) (int, error) {
	w.WriteHeaderNow()
	w.size += len(body)
	return len(body), nil
}
func (w *supplierTaskResponseWriter) WriteString(body string) (int, error) {
	return w.Write([]byte(body))
}
func (w *supplierTaskResponseWriter) Status() int   { return w.status }
func (w *supplierTaskResponseWriter) Size() int     { return w.size }
func (w *supplierTaskResponseWriter) Written() bool { return w.size >= 0 }
func (w *supplierTaskResponseWriter) Flush()        { w.WriteHeaderNow() }

func writeSupplierTaskAccepted(c *gin.Context, info *relaycommon.RelayInfo) {
	modelName := info.RequestedModelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/videos") {
		video := dto.NewOpenAIVideo()
		video.ID, video.Model, video.CreatedAt = info.PublicTaskID, modelName, time.Now().Unix()
		c.JSON(http.StatusOK, video)
		return
	}
	c.JSON(http.StatusOK, dto.TaskResponse[any]{Code: dto.TaskSuccessCode, Data: map[string]any{
		"task_id": info.PublicTaskID, "status": "queued", "model": modelName,
	}})
}

func supplierTaskVideo(task *model.Task) *dto.OpenAIVideo {
	video := dto.NewOpenAIVideo()
	video.ID, video.Model = task.TaskID, task.PublicModelName()
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt, video.CompletedAt = task.CreatedAt, task.FinishTime
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Code: "task_failed", Message: "Task failed"}
	}
	return video
}

func supplierTaskVideoBody(task *model.Task) ([]byte, error) {
	return common.Marshal(supplierTaskVideo(task))
}
