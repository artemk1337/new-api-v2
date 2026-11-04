package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSyncUpstreamModelsPreservesExactNameForAliases(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	local := model.Model{
		ModelName: "sync-alias-canonical", Aliases: model.ModelAliases{"sync-cursor-alias"},
		Description: "local description", Status: 1, SyncOfficial: 1,
	}
	require.NoError(t, local.Insert())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any = []upstreamVendor{}
		if strings.HasSuffix(r.URL.Path, "/models.json") {
			data = []upstreamModel{{ModelName: local.ModelName, NameRule: model.NameRulePrefix, Description: "upstream description"}}
		}
		body, err := common.Marshal(gin.H{"success": true, "data": data})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(upstream.Close)
	t.Setenv("SYNC_UPSTREAM_BASE", upstream.URL)

	for _, tc := range []struct {
		name, fields, description string
		updated                   int
	}{
		{"reject incompatible rule", `["name_rule","description"]`, "local description", 0},
		{"allow other metadata", `["description"]`, "upstream description", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync", strings.NewReader(`{"overwrite":[{"model_name":"sync-alias-canonical","fields":`+tc.fields+`}]} `))
			ctx.Request.Header.Set("Content-Type", "application/json")
			SyncUpstreamModels(ctx)
			var body struct {
				Success bool `json:"success"`
				Data    struct {
					Updated int      `json:"updated_models"`
					Skipped []string `json:"skipped_models"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.True(t, body.Success, response.Body.String())
			require.Equal(t, tc.updated, body.Data.Updated)
			if tc.updated == 0 {
				require.Contains(t, body.Data.Skipped, local.ModelName)
			}
			var saved model.Model
			require.NoError(t, db.First(&saved, local.Id).Error)
			require.Equal(t, model.NameRuleExact, saved.NameRule)
			require.Equal(t, local.Aliases, saved.Aliases)
			require.Equal(t, tc.description, saved.Description)
		})
	}
}
