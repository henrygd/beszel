//go:build !linux

package agent

type fail2banManager struct{}

func newFail2banManager() *fail2banManager { return nil }

func (m *fail2banManager) getBannedCounts() map[string]uint32 { return nil }
