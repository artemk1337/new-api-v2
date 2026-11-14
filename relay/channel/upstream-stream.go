package channel

import (
	"context"
	"io"
)

type upstreamStreamBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *upstreamStreamBody) Close() error {
	// The stream scanner closes this body after draining final usage. Cancel
	// here so transport shutdown follows the same accounting grace period.
	b.cancel()
	return b.ReadCloser.Close()
}
