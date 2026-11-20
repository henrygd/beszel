package hwnames

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sample = `# comment
1022  Advanced Micro Devices, Inc. [AMD]
	1452  Family 17h (Models 00h-1fh) PCIe Dummy Host Bridge
		1022 1452  subsystem line is skipped
10de  NVIDIA Corporation
	2484  GA104 [GeForce RTX 3070]
C 03  Display controller
	00  VGA compatible controller
		00  VGA controller
AT 0001  usb.ids-only section is skipped
`

func TestParse(t *testing.T) {
	n := parse(bufio.NewScanner(strings.NewReader(sample)))
	assert.Equal(t, "NVIDIA Corporation", n["10de"])
	assert.Equal(t, "GA104 [GeForce RTX 3070]", n["10de:2484"])
	assert.Equal(t, "Family 17h (Models 00h-1fh) PCIe Dummy Host Bridge", n["1022:1452"])
	assert.Equal(t, "VGA compatible controller", n["class:0300"])
	assert.Len(t, n, 5)
}
