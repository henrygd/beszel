//go:build linux

package hardware

import (
	"io"
	"os"
	"path"
	"strings"
)

const (
	maxAttribute = 4 << 10  // sysfs text attributes are at most a page
	maxEDID      = 32 << 10 // base block plus up to 255 extensions
)

// sysfs reads under a fixed root through os.Root, so no path (or symlink in
// the tree) can resolve outside it. Every read is size-capped and read-only.
type sysfs struct {
	root *os.Root
}

func openSysfs(dir string) (sysfs, error) {
	r, err := os.OpenRoot(dir)
	return sysfs{root: r}, err
}

func (s sysfs) close() { _ = s.root.Close() }

// bytes reads at most limit bytes of name.
func (s sysfs) bytes(name string, limit int64) ([]byte, error) {
	f, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// text returns a trimmed attribute, or "" when it is missing or unreadable.
func (s sysfs) text(name string) string {
	b, err := s.bytes(name, maxAttribute)
	if err != nil {
		return ""
	}
	return clean(string(b))
}

// hex returns a "0x1234"-style attribute without its prefix.
func (s sysfs) hex(name string) string {
	return strings.TrimPrefix(s.text(name), "0x")
}

// list returns the entry names of a directory, or nil.
func (s sysfs) list(dir string) []string {
	f, err := s.root.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil
	}
	return names
}

// link returns the target of a symlink, or "".
func (s sysfs) link(name string) string {
	target, err := s.root.Readlink(name)
	if err != nil {
		return ""
	}
	return target
}

// linkBase returns the last element of a symlink target (a driver or device name).
func (s sysfs) linkBase(name string) string {
	if target := s.link(name); target != "" {
		return path.Base(target)
	}
	return ""
}

func (s sysfs) exists(name string) bool {
	_, err := s.root.Lstat(name)
	return err == nil
}
