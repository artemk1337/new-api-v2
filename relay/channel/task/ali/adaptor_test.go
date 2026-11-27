package ali

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestVideoAPIURLs(t *testing.T) {
	for _, tc := range []struct {
		baseURL string
		want    string
	}{
		{"https://api.openlux.ai/", "https://api.openlux.ai/alibailian/api/v1"},
		{"https://api.openlux.ai/alibailian", "https://api.openlux.ai/alibailian/api/v1"},
		{"https://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com/api/v1"},
	} {
		t.Run(tc.baseURL, func(t *testing.T) {
			adaptor := TaskAdaptor{baseURL: tc.baseURL}
			got, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
			require.NoError(t, err)
			require.Equal(t, tc.want+"/services/aigc/video-generation/video-synthesis", got)
			require.Equal(t, tc.want, videoAPIBaseURL(tc.baseURL))
		})
	}
}
