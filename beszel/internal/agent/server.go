package agent

import (
	"encoding/json"
	"log"

	sshServer "github.com/gliderlabs/ssh"
)

func (a *Agent) startServer() {
	sshServer.Handle(a.handleSession)

	log.Printf("Starting SSH server on %s", a.addr)
	if err := sshServer.ListenAndServe(a.addr, nil, sshServer.NoPty(),
		sshServer.PublicKeyAuth(func(ctx sshServer.Context, key sshServer.PublicKey) bool {
			allowed, _, _, _, _ := sshServer.ParseAuthorizedKey(a.pubKey)
			return sshServer.KeysEqual(key, allowed)
		}),
	); err != nil {
		log.Fatal(err)
	}
}

func (a *Agent) handleSession(s sshServer.Session) {
	stats := a.gatherStats()
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(stats); err != nil {
		log.Println("Error encoding stats:", err.Error())
		s.Exit(1)
		return
	}
	s.Exit(0)
}
