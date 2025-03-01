package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	sshServer "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

type ServerOptions struct {
	Addr    string
	Network string
	Keys    []ssh.PublicKey
}

func (a *Agent) StartServer(opts ServerOptions) error {
	sshServer.Handle(a.handleSession)

	slog.Info("Starting SSH server", "addr", opts.Addr, "network", opts.Network)

	switch opts.Network {
	case "unix":
		// remove existing socket file if it exists
		if err := os.Remove(opts.Addr); err != nil && !os.IsNotExist(err) {
			return err
		}
	default:
		// prefix with : if only port was provided
		if !strings.Contains(opts.Addr, ":") {
			opts.Addr = ":" + opts.Addr
		}
	}

	// Listen on the address
	ln, err := net.Listen(opts.Network, opts.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	// Start SSH server on the listener
	err = sshServer.Serve(ln, nil, sshServer.NoPty(),
		sshServer.PublicKeyAuth(func(ctx sshServer.Context, key sshServer.PublicKey) bool {
			for _, pubKey := range opts.Keys {
				if sshServer.KeysEqual(key, pubKey) {
					return true
				}
			}
			return false
		}),
	)
	if err != nil {
		return err
	}
	return nil
}

func (a *Agent) handleSession(s sshServer.Session) {
	// slog.Debug("connection", "remoteaddr", s.RemoteAddr(), "user", s.User())
	stats := a.gatherStats()
	if err := json.NewEncoder(s).Encode(stats); err != nil {
		slog.Error("Error encoding stats", "err", err, "stats", stats)
		s.Exit(1)
	}
	s.Exit(0)
}

// ParseKeys parses a string containing SSH public keys in authorized_keys format.
// It returns a slice of ssh.PublicKey and an error if any key fails to parse.
func ParseKeys(input string) ([]ssh.PublicKey, error) {
	var parsedKeys []ssh.PublicKey

	for line := range strings.Lines(input) {
		line = strings.TrimSpace(line)

		// Skip empty lines or comments
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the key
		parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("failed to parse key: %s, error: %w", line, err)
		}

		// Append the parsed key to the list
		parsedKeys = append(parsedKeys, parsedKey)
	}

	return parsedKeys, nil
}
