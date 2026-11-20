//go:build darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"time"

	"howett.net/plist"
)

// diskutilTimeout bounds each lookup. diskutil blocks until the drive answers,
// which took ~7s for a sleeping external HDD, so leave room for slow spin-up.
const diskutilTimeout = 15 * time.Second

var wholeDiskPattern = regexp.MustCompile(`^disk[0-9]+`)

// diskutilPhysicalDisk returns the whole physical disk backing an APFS volume
// (e.g. disk13s1 in container disk13 on store disk12s2 -> disk12), or "" if
// the device is not APFS or diskutil fails.
func diskutilPhysicalDisk(device string) string {
	ctx, cancel := context.WithTimeout(context.Background(), diskutilTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", "info", "-plist", device).Output()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("diskutil timed out after %s", diskutilTimeout)
	}
	if err != nil {
		slog.Warn("APFS disk I/O lookup failed", "device", device, "err", err)
		return ""
	}
	physical, err := parseApfsPhysicalDisk(out)
	if err != nil {
		slog.Debug("diskutil info parse failed", "device", device, "err", err)
		return ""
	}
	return physical
}

// parseApfsPhysicalDisk extracts the whole disk of the first APFS physical
// store from `diskutil info -plist` output. For multi-store containers
// (e.g. Fusion Drive) only the first store is used.
func parseApfsPhysicalDisk(out []byte) (string, error) {
	var info struct {
		APFSPhysicalStores []struct {
			APFSPhysicalStore string `plist:"APFSPhysicalStore"`
		} `plist:"APFSPhysicalStores"`
	}
	if _, err := plist.Unmarshal(out, &info); err != nil {
		return "", err
	}
	if len(info.APFSPhysicalStores) == 0 {
		return "", nil
	}
	return wholeDiskPattern.FindString(info.APFSPhysicalStores[0].APFSPhysicalStore), nil
}
