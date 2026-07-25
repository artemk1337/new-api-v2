package dify

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func difyUploadTestContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func difyUploadTestMedia(data string) dto.MediaContent {
	return dto.MediaContent{
		Type: dto.ContentTypeImageURL,
		ImageUrl: &dto.MessageImageUrl{
			Url:      data,
			MimeType: "image/png",
		},
	}
}

func classifyDifyUploadError(info *relaycommon.RelayInfo, err error) types.AttemptFinancialOutcome {
	apiErr := types.NewError(err, types.ErrorCodeDoRequestFailed)
	return service.ClassifyAttempt(info, apiErr)
}

func TestUploadDifyFileMalformedBase64IsPreDispatch(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "http://unused.invalid"},
	}

	_, err := uploadDifyFile(difyUploadTestContext(), info, "user", difyUploadTestMedia("%%%"))

	require.Error(t, err)
	assert.False(t, info.AttemptDispatched)
	assert.Equal(t, types.AttemptFinancialOutcomeNonBillable, classifyDifyUploadError(info, err))
}

func TestUploadDifyFilePostDispatchFailuresAreAmbiguous(t *testing.T) {
	service.InitHttpClient()
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{`},
		{name: "missing file id", body: `{"id":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL},
			}

			_, err := uploadDifyFile(difyUploadTestContext(), info, "user", difyUploadTestMedia("aW1hZ2U="))

			require.Error(t, err)
			assert.True(t, info.AttemptDispatched)
			assert.False(t, info.AttemptHasBillingEvidence)
			assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, classifyDifyUploadError(info, err))
		})
	}
}

func TestUploadDifyFileTransportFailureIsAmbiguous(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL},
	}

	_, err := uploadDifyFile(difyUploadTestContext(), info, "user", difyUploadTestMedia("aW1hZ2U="))

	require.Error(t, err)
	assert.True(t, info.AttemptDispatched)
	assert.False(t, info.AttemptHasBillingEvidence)
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, classifyDifyUploadError(info, err))
}

func TestUploadDifyFileSuccessMarksBillingEvidence(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-123"}`))
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL},
	}

	file, err := uploadDifyFile(difyUploadTestContext(), info, "user", difyUploadTestMedia("aW1hZ2U="))

	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Equal(t, "file-123", file.UploadFileId)
	assert.True(t, info.AttemptDispatched)
	assert.True(t, info.AttemptHasBillingEvidence)
	assert.Equal(t, types.AttemptFinancialOutcomeBillable, service.ClassifyAttempt(info, nil))
}
