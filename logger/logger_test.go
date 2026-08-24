package logger

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLogHelperConcurrentRotationTrigger(t *testing.T) {
	common.LogWriterMu.Lock()
	previousWriter, previousErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = io.Discard, io.Discard
	common.LogWriterMu.Unlock()
	previousCount := logCount.Load()
	previousWorking := setupLogWorking.Load()
	previousSchedule := scheduleLogSetup
	var rotations atomic.Int64
	scheduleLogSetup = func() { rotations.Add(1) }
	logCount.Store(maxLogCount)
	setupLogWorking.Store(false)
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = previousWriter, previousErrorWriter
		common.LogWriterMu.Unlock()
		logCount.Store(previousCount)
		setupLogWorking.Store(previousWorking)
		scheduleLogSetup = previousSchedule
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			LogInfo(context.Background(), "concurrent logger rotation trigger")
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), rotations.Load())
	require.LessOrEqual(t, logCount.Load(), int64(8))
}
