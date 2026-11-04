package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierTaskPublicResponseHidesProcurementDetails(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public", ChannelId: 73, Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			RequestedModelName: "client-model", OriginModelName: "source-alias", UpstreamModelName: "source-alias", BillingModelName: "internal-price-model",
		},
		FailReason:  "https://supplier.example/internal-error",
		Data:        []byte(`{"provider":"private-source","url":"https://supplier.example/output"}`),
		PrivateData: model.TaskPrivateData{ResultURL: "https://supplier.example/output", BillingContext: &model.TaskBillingContext{SupplierID: 17}},
	}
	public, err := common.Marshal(TaskModel2Dto(task))
	require.NoError(t, err)
	for _, hidden := range []string{"channel_id", "source-alias", "internal-price-model", "supplier.example", "private-source", "supplier_id"} {
		require.NotContains(t, string(public), hidden)
	}
	require.Contains(t, string(public), "client-model")
	require.Contains(t, string(public), "/v1/videos/task_public/content")
	admin := TaskModel2AdminDto(task)
	require.Equal(t, 73, admin.ChannelId)
	require.Equal(t, task.Data, admin.Data)
	require.Equal(t, task.FailReason, admin.FailReason)
	video, err := supplierTaskVideoBody(task)
	require.NoError(t, err)
	require.Contains(t, string(video), "client-model")
	require.NotContains(t, string(video), "source-alias")
	require.NotContains(t, string(video), "supplier.example")
}

func TestSupplierTaskAdaptorCannotWriteNativePayloadOrHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	original := ctx.Writer
	id, raw, taskErr := parseSupplierTaskResponse(ctx, func() (string, []byte, *dto.TaskError) {
		ctx.Header("X-Supplier", "private-source")
		ctx.JSON(http.StatusOK, map[string]any{"model": "source-alias", "url": "https://supplier.example/output"})
		return "source-task-id", []byte(`{"native":true}`), nil
	})
	require.Nil(t, taskErr)
	require.Equal(t, "source-task-id", id)
	require.Equal(t, `{"native":true}`, string(raw))
	require.Same(t, original, ctx.Writer)
	require.Empty(t, recorder.Body.String())
	require.Empty(t, recorder.Header().Get("X-Supplier"))
	info := &relaycommon.RelayInfo{RequestedModelName: "client-model", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
	writeSupplierTaskAccepted(ctx, info)
	require.Contains(t, recorder.Body.String(), "client-model")
	require.Contains(t, recorder.Body.String(), "task_public")
	require.NotContains(t, recorder.Body.String(), "source-")
}

func TestSupplierTaskSnapshotPersistsPrivateProvenance(t *testing.T) {
	private := model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
		SupplierID: 71, ProfileID: 72, PriceRevision: 4,
		PriceSnapshot: &types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 71, ProfileID: 72, Revision: 4, BillingMode: types.SupplierBillingModeFixed}},
	}}
	encoded, err := common.Marshal(private)
	require.NoError(t, err)
	var restored model.TaskPrivateData
	require.NoError(t, common.Unmarshal(encoded, &restored))
	require.Equal(t, 71, restored.BillingContext.SupplierID)
	require.Equal(t, 72, restored.BillingContext.ProfileID)
	require.Equal(t, int64(4), restored.BillingContext.PriceRevision)
	require.True(t, (&model.Task{PrivateData: restored}).UsesSupplierPricing())
}
