package agent

import (
	"encoding/json"
	"log/slog"
	"os"

	sshServer "github.com/gliderlabs/ssh"
)

func (a *Agent) startServer(pubKey []byte, addr string) {
	sshServer.Handle(a.handleSession)

	slog.Info("Starting SSH server", "address", addr)
	if err := sshServer.ListenAndServe(addr, nil, sshServer.NoPty(),
		sshServer.PublicKeyAuth(func(ctx sshServer.Context, key sshServer.PublicKey) bool {
			allowed, _, _, _, _ := sshServer.ParseAuthorizedKey(pubKey)
			return sshServer.KeysEqual(key, allowed)
		}),
	); err != nil {
		slog.Error("Error starting SSH server", "err", err)
		os.Exit(1)
	}
}

func (a *Agent) handleSession(s sshServer.Session) {
	stats := a.gatherStats()
	if err := json.NewEncoder(s).Encode(stats); err != nil {
		slog.Error("Error encoding stats", "err", err, "stats", stats)
		s.Exit(1)
		return
	}
	s.Exit(0)
}
