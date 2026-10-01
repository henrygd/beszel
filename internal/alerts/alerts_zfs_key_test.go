//go:build testing

package alerts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZfsDiskAlertKeyIsNamespaced(t *testing.T) {
	assert.Equal(t, "zfs:tank", zfsDiskAlertKey("tank"))
	assert.Equal(t, "Usage of storage pool tank", diskAlertDescriptor(zfsDiskAlertKey("tank")))
	assert.Equal(t, "Usage of tank", diskAlertDescriptor("tank"))
}
