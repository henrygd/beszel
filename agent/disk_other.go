//go:build !darwin

package agent

// diskutilPhysicalDisk is only implemented on darwin, where APFS volumes need
// mapping to their physical disk for I/O.
func diskutilPhysicalDisk(string) string { return "" }
