package helper

import (
	"context"

	"github.com/gin-gonic/gin"
)

// streamDrainWriter lets adapters collect usage without writing to a disconnected client.
type streamDrainWriter struct {
	gin.ResponseWriter
	clientContext context.Context
}

func (w *streamDrainWriter) Write(data []byte) (int, error) {
	if w.clientContext.Err() != nil {
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *streamDrainWriter) WriteString(data string) (int, error) {
	if w.clientContext.Err() != nil {
		return len(data), nil
	}
	return w.ResponseWriter.WriteString(data)
}

func (w *streamDrainWriter) WriteHeader(code int) {
	if w.clientContext.Err() == nil {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *streamDrainWriter) WriteHeaderNow() {
	if w.clientContext.Err() == nil {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *streamDrainWriter) Flush() {
	if w.clientContext.Err() == nil {
		w.ResponseWriter.Flush()
	}
}
