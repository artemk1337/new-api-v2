package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManageUserDoesNotEnableVerificationFrozenUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:manage-user-verification?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))

	originalDB, originalLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = map[string]string{
		setting.AccountVerificationEnabledOption:            "true",
		setting.AccountVerificationProvidersOption:          "email",
		setting.AccountVerificationFreezeDelayMinutesOption: "60",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	frozen := &model.User{
		Username:               "manage-user-verification-frozen",
		Password:               "password",
		AffCode:                "manage-user-verification-frozen",
		Role:                   common.RoleCommonUser,
		Status:                 model.UserStatusVerificationFrozen,
		VerificationRequiredAt: 100,
		VerificationFrozenAt:   200,
	}
	require.NoError(t, db.Create(frozen).Error)
	require.True(t, model.AccountVerificationBlocksManualEnable(frozen))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", bytes.NewBufferString(`{"id":`+strconv.Itoa(frozen.Id)+`,"action":"enable"}`))
	ctx.Set("role", common.RoleRootUser)
	ManageUser(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var persisted model.User
	require.NoError(t, db.First(&persisted, frozen.Id).Error)
	assert.Equal(t, model.UserStatusVerificationFrozen, persisted.Status)
	assert.Equal(t, int64(100), persisted.VerificationRequiredAt)
	assert.Equal(t, int64(200), persisted.VerificationFrozenAt)
}
