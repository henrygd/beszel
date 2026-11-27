//go:build windows

package agent

// hwmonRoot is empty on Windows, where fan speeds come from LibreHardwareMonitor.
const hwmonRoot = ""

// platformFans returns the fan speeds read by the LHM helper along with
// temperatures (LHM=true). It's empty when LHM is off or reports no fans.
var platformFans = lhmFans.take
