package agent

// Seccomp-safe executable lookup for Android.
//
// Since Go 1.24, exec.LookPath probes executables with the faccessat2
// syscall (439). Android's seccomp filter only whitelisted that syscall
// from Android 12 onwards; on older devices (e.g. Android 8) calling it
// kills the process with SIGSYS: bad system call. The agent's package
// update manager and GPU tool detection run LookPath at startup, so the
// agent crashed on boot there.
//
// safeLookPath replicates LookPath by scanning PATH with os.Stat, which
// uses the ancient newfstatat syscall that every Android seccomp filter
// allows. Behavior matches exec.LookPath closely enough for the agent's
// "does this tool exist" checks.

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// safeLookPath searches PATH for an executable file, or verifies a direct
// path, without using faccessat2. Mirrors os/exec lp_unix.go semantics.
func safeLookPath(file string) (string, error) {
	if strings.Contains(file, "/") {
		if err := androidExecutableCheck(file); err != nil {
			return "", err
		}
		return file, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, file)
		if err := androidExecutableCheck(path); err == nil {
			return path, nil
		}
	}
	return "", fs.ErrNotExist
}

// androidExecutableCheck reports whether path exists and is a regular,
// executable file, using only os.Stat (newfstatat).
func androidExecutableCheck(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fs.ErrInvalid
	}
	if fi.Mode()&0o111 == 0 {
		return fs.ErrPermission
	}
	return nil
}

// safeCommand is a seccomp-safe exec.Command: it resolves the binary with
// safeLookPath (os.Stat based) instead of exec.LookPath, whose faccessat2
// probe kills the process on Android < 12. If the binary cannot be found,
// behavior defers to exec.Command (the error surfaces on Run, exactly like
// a missing binary would on a normal Linux host).
func safeCommand(name string, args ...string) *exec.Cmd {
	if path, err := safeLookPath(name); err == nil {
		return &exec.Cmd{
			Path: path,
			Args: append([]string{name}, args...),
		}
	}
	return exec.Command(name, args...)
}
