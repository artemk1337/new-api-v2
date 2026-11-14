package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath2RelayModeImageGenerations(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/pg/images/generations"} {
		t.Run(path, func(t *testing.T) {
			assert.Equal(t, RelayModeImagesGenerations, Path2RelayMode(path))
		})
	}
}
