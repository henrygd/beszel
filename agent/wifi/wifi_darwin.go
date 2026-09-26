//go:build darwin

package wifi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

// JXA exposes the system CoreWLAN framework without cgo, private airport tools,
// sudo, or scanning nearby networks. SSID can be redacted by macOS privacy rules.
const coreWLANScript = `ObjC.import('CoreWLAN');
var result = {};
var interfaces = $.CWWiFiClient.sharedWiFiClient.interfaces;
if (interfaces) {
 for (var i = 0; i < interfaces.count; i++) {
  var iface = interfaces.objectAtIndex(i);
  if (!iface.powerOn || Number(iface.interfaceMode) !== 1) continue;
  var name = ObjC.unwrap(iface.interfaceName);
  if (!name) continue;
  var reading = {};
  var ssid = ObjC.unwrap(iface.ssid);
  if (ssid) reading.s = ssid;
  var signal = Number(iface.rssiValue);
  if (signal >= -150 && signal < 0) reading.r = signal;
  result[name] = reading;
 }
}
JSON.stringify(result);`

func collect(ctx context.Context) map[string]system.WiFi {
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", coreWLANScript)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	cmd.WaitDelay = 100 * time.Millisecond
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var result map[string]system.WiFi
	if json.Unmarshal(output, &result) != nil {
		return nil
	}
	return result
}
