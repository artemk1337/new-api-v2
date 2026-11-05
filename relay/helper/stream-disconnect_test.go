package helper

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamScannerDrainsDisconnectedClientWithoutWriting(t *testing.T) {
	c, resp, info := setupStreamTest(t, strings.NewReader("data: content\ndata: usage\ndata: [DONE]\n"))
	info.DisablePing = true
	clientContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientContext)
	cancel()
	originalWriter := c.Writer
	var received []string

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		received = append(received, data)
		// Cover both direct adapter writes and the shared SSE helpers.
		_, err := c.Writer.Write([]byte(data))
		require.NoError(t, err)
		_, err = c.Writer.WriteString(data)
		require.NoError(t, err)
		c.Writer.WriteHeaderNow()
		c.Writer.Flush()
		if err := StringData(c, data); err != nil {
			sr.Stop(err)
		}
	})

	require.Equal(t, []string{"content", "usage"}, received)
	require.Same(t, originalWriter, c.Writer)
	require.False(t, c.Writer.Written())
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	require.False(t, info.StreamStatus.HasErrors())
}

type blockedPingWriter struct {
	gin.ResponseWriter
	started chan struct{}
	release chan struct{}
}

func (w *blockedPingWriter) Write(data []byte) (int, error) {
	close(w.started)
	<-w.release
	return len(data), nil
}

func TestStreamScannerDisconnectWaitsForPendingPing(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = oldEnabled, oldInterval
	})
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	c, resp, info := setupStreamTest(t, reader)
	resp.Body = reader
	clientContext, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(clientContext)
	pingWriter := &blockedPingWriter{ResponseWriter: c.Writer, started: make(chan struct{}), release: make(chan struct{})}
	c.Writer = pingWriter
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(pingWriter.release) }) }
	t.Cleanup(release)
	finished := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(string, *StreamResult) {})
		close(finished)
	}()
	select {
	case <-pingWriter.started:
	case <-time.After(3 * time.Second):
		t.Fatal("ping did not start")
	}
	cancel()
	select {
	case <-finished:
		t.Fatal("handler returned while a ping write was still running")
	case <-time.After(streamDisconnectDrainTimeout + 50*time.Millisecond):
	}
	release()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after the pending ping completed")
	}
	require.Same(t, pingWriter, c.Writer)
}

func TestStreamScannerDisconnectDrainClosesBlockedUpstream(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	c, resp, info := setupStreamTest(t, reader)
	resp.Body = reader
	info.DisablePing = true
	clientContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientContext)
	cancel()

	start := time.Now()
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		t.Error("unexpected upstream event")
	})

	require.GreaterOrEqual(t, time.Since(start), streamDisconnectDrainTimeout)
	require.Less(t, time.Since(start), streamDisconnectDrainTimeout+2*time.Second)
	_, err := writer.Write([]byte("data: late usage\n"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
}
