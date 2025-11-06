//go:build !windows

package agent

import "errors"

func ensureEmbeddedSmartctl() (string, error) {
	return "", errors.ErrUnsupported
}
