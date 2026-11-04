package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

type taskTransportPollingAdaptor struct{ calls atomic.Int32 }

func (a *taskTransportPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *taskTransportPollingAdaptor) FetchTask(baseURL, key string, _ map[string]any, _ string) (*http.Response, error) {
	a.calls.Add(1)
	request, err := http.NewRequest(http.MethodGet, baseURL+"/poll", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	return http.DefaultClient.Do(request)
}
func (a *taskTransportPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: string(model.TaskStatusInProgress)}, nil
}
func (a *taskTransportPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestTaskSubmitRaceCannotPollNewTransportOrRefund(t *testing.T) {
	for index, mutation := range []string{"supplier", "url", "key", "type"} {
		t.Run(mutation, func(t *testing.T) {
			truncate(t)
			var requests atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); _, _ = w.Write([]byte("{}")) }))
			defer target.Close()
			supplierID, profileID := 11, 12
			channel := &model.Channel{Id: 1200 + index, Type: constant.ChannelTypeSora, BaseURL: &target.URL, Key: "frozen-key", SupplierID: &supplierID, SupplierProfileID: &profileID}
			require.NoError(t, model.DB.Create(channel).Error)
			fingerprint := channel.TaskTransportFingerprint()
			// The provider has accepted a request, but its local Task row does not
			// exist yet. The administrative update can therefore win this race.
			switch mutation {
			case "supplier":
				nextSupplier, nextProfile := 21, 22
				channel.SupplierID, channel.SupplierProfileID = &nextSupplier, &nextProfile
			case "url":
				changed := target.URL + "/new-host"
				channel.BaseURL = &changed
			case "key":
				channel.Key = "replacement-key"
			case "type":
				channel.Type = constant.ChannelTypeGemini
			}
			require.NoError(t, model.DB.Save(channel).Error)
			task := &model.Task{TaskID: "task_raced_" + mutation, Platform: constant.TaskPlatform("transport"), ChannelId: channel.Id, Status: model.TaskStatusInProgress, Progress: "20%", Quota: 125,
				SubmitTime: time.Now().Add(-2 * time.Minute).Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-upstream-id", Key: "frozen-key", TransportFingerprint: fingerprint, BillingContext: &model.TaskBillingContext{SupplierID: 11, ProfileID: 12}}}
			require.NoError(t, model.DB.Create(task).Error)
			adaptor := &taskTransportPollingAdaptor{}
			previousFactory := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
			t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
			require.NoError(t, UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{channel.Id: {"private-upstream-id"}}, map[string]*model.Task{"private-upstream-id": task}))
			if mutation == "url" {
				require.Error(t, updateSunoTasks(context.Background(), channel.Id, []string{"private-upstream-id"}, map[string]*model.Task{"private-upstream-id": task}))
			}
			previousTimeout := constant.TaskTimeoutMinutes
			constant.TaskTimeoutMinutes = 1
			sweepTimedOutTasks(context.Background())
			constant.TaskTimeoutMinutes = previousTimeout
			var persisted model.Task
			require.NoError(t, model.DB.First(&persisted, task.ID).Error)
			require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), persisted.Status)
			require.Equal(t, 125, persisted.Quota)
			require.Zero(t, adaptor.calls.Load())
			require.Zero(t, requests.Load())
		})
	}
}
