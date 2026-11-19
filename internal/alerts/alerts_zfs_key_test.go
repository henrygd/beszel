//go:build testing

package alerts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZfsDiskAlertKeyIsNamespaced(t *testing.T) {
	assert.Equal(t, "zfs:tank", zfsDiskAlertKey("tank"))
	assert.Equal(t, "存储池 tank 的使用率", diskAlertDescriptor(zfsDiskAlertKey("tank")))
	assert.Equal(t, "tank 的使用率", diskAlertDescriptor("tank"))
}
