//go:build !linux

package agent

// statMount has no portable equivalent outside Linux, so discovery falls back
// to matching device names and comparing usage.
var statMount = func(string) mountInfo { return mountInfo{} }
