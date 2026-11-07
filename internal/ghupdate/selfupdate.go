//go:build !no_self_update

package ghupdate

// SelfUpdateDisabled reports whether self-update was disabled at build time
// with the no_self_update build tag.
const SelfUpdateDisabled = false
