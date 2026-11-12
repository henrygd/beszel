//go:build no_self_update

package ghupdate

// SelfUpdateDisabled reports whether self-update was disabled at build time
// with the no_self_update build tag. Package managers that handle upgrades
// themselves (e.g. Homebrew) should build with this tag.
const SelfUpdateDisabled = true
