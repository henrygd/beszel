package agent

import (
	"beszel"
	"beszel/internal/common"
	"beszel/internal/entities/system"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// ServerOptions contains configuration options for starting the SSH server.
type ServerOptions struct {
	Addr    string            // Network address to listen on (e.g., ":45876" or "/path/to/socket")
	Network string            // Network type ("tcp" or "unix")
	Keys    []gossh.PublicKey // SSH public keys for authentication
}

// hubVersions caches hub versions by session ID to avoid repeated parsing.
var hubVersions map[string]semver.Version

// StartServer starts the SSH server with the provided options.
// It configures the server with secure defaults, sets up authentication,
// and begins listening for connections. Returns an error if the server
// is already running or if there's an issue starting the server.
func (a *Agent) StartServer(opts ServerOptions) error {
	if a.server != nil {
		return errors.New("server already started")
	}

	slog.Info("Starting SSH server", "addr", opts.Addr, "network", opts.Network)

	if opts.Network == "unix" {
		// remove existing socket file if it exists
		if err := os.Remove(opts.Addr); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// start listening on the address
	ln, err := net.Listen(opts.Network, opts.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	// base config (limit to allowed algorithms)
	config := &gossh.ServerConfig{
		ServerVersion: fmt.Sprintf("SSH-2.0-%s_%s", beszel.AppName, beszel.Version),
	}
	config.KeyExchanges = common.DefaultKeyExchanges
	config.MACs = common.DefaultMACs
	config.Ciphers = common.DefaultCiphers

	// set default handler
	ssh.Handle(a.handleSession)

	a.server = &ssh.Server{
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return config
		},
		// check public key(s)
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			remoteAddr := ctx.RemoteAddr()
			for _, pubKey := range opts.Keys {
				if ssh.KeysEqual(key, pubKey) {
					slog.Info("SSH connected", "addr", remoteAddr)
					return true
				}
			}
			slog.Warn("Invalid SSH key", "addr", remoteAddr)
			return false
		},
		// disable pty
		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return false
		},
		// close idle connections after 70 seconds
		IdleTimeout: 70 * time.Second,
	}

	// Start SSH server on the listener
	return a.server.Serve(ln)
}

// getHubVersion retrieves and caches the hub version for a given session.
// It extracts the version from the SSH client version string and caches
// it to avoid repeated parsing. Returns a zero version if parsing fails.
func (a *Agent) getHubVersion(sessionId string, sessionCtx ssh.Context) semver.Version {
	if hubVersions == nil {
		hubVersions = make(map[string]semver.Version, 1)
	}
	hubVersion, ok := hubVersions[sessionId]
	if ok {
		return hubVersion
	}
	// Extract hub version from SSH client version
	clientVersion := sessionCtx.Value(ssh.ContextKeyClientVersion)
	if versionStr, ok := clientVersion.(string); ok {
		hubVersion, _ = extractHubVersion(versionStr)
	}
	hubVersions[sessionId] = hubVersion
	return hubVersion
}

// handleSession handles an incoming SSH session by gathering system statistics
// and sending them to the hub. It signals connection events, determines the
// appropriate encoding format based on hub version, and exits with appropriate
// status codes.
func (a *Agent) handleSession(s ssh.Session) {
	a.connectionManager.eventChan <- SSHConnect

	sessionCtx := s.Context()
	sessionID := sessionCtx.SessionID()

	hubVersion := a.getHubVersion(sessionID, sessionCtx)

	stats := a.gatherStats(sessionID)

	err := a.writeToSession(s, stats, hubVersion)
	if err != nil {
		slog.Error("Error encoding stats", "err", err, "stats", stats)
		s.Exit(1)
	} else {
		s.Exit(0)
	}
}

// writeToSession encodes and writes system statistics to the session.
// It chooses between CBOR and JSON encoding based on the hub version,
// using CBOR for newer versions and JSON for legacy compatibility.
func (a *Agent) writeToSession(w io.Writer, stats *system.CombinedData, hubVersion semver.Version) error {
	if hubVersion.GTE(beszel.MinVersionCbor) {
		return cbor.NewEncoder(w).Encode(stats)
	}
	return json.MarshalEncode(jsontext.NewEncoder(w), stats)
}

// extractHubVersion extracts the beszel version from SSH client version string.
// Expected format: "SSH-2.0-beszel_X.Y.Z" or "beszel_X.Y.Z"
func extractHubVersion(versionString string) (semver.Version, error) {
	_, after, _ := strings.Cut(versionString, "_")
	return semver.Parse(after)
}

// ParseKeys parses a string containing SSH public keys in authorized_keys format.
// It returns a slice of ssh.PublicKey and an error if any key fails to parse.
func ParseKeys(input string) ([]gossh.PublicKey, error) {
	var parsedKeys []gossh.PublicKey
	for line := range strings.Lines(input) {
		line = strings.TrimSpace(line)
		// Skip empty lines or comments
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse the key
		parsedKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("failed to parse key: %s, error: %w", line, err)
		}
		parsedKeys = append(parsedKeys, parsedKey)
	}
	return parsedKeys, nil
}

// GetAddress determines the network address to listen on from various sources.
// It checks the provided address, then environment variables (LISTEN, PORT),
// and finally defaults to ":45876".
func GetAddress(addr string) string {
	if addr == "" {
		addr, _ = GetEnv("LISTEN")
	}
	if addr == "" {
		// Legacy PORT environment variable support
		addr, _ = GetEnv("PORT")
	}
	if addr == "" {
		return ":45876"
	}
	// prefix with : if only port was provided
	if GetNetwork(addr) != "unix" && !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	return addr
}

// GetNetwork determines the network type based on the address format.
// It checks the NETWORK environment variable first, then infers from
// the address format: addresses starting with "/" are "unix", others are "tcp".
func GetNetwork(addr string) string {
	if network, ok := GetEnv("NETWORK"); ok && network != "" {
		return network
	}
	if strings.HasPrefix(addr, "/") {
		return "unix"
	}
	return "tcp"
}

// StopServer stops the SSH server if it's running.
// It returns an error if the server is not running or if there's an error stopping it.
func (a *Agent) StopServer() error {
	if a.server == nil {
		return errors.New("SSH server not running")
	}

	slog.Info("Stopping SSH server")
	_ = a.server.Close()
	a.server = nil
	a.connectionManager.eventChan <- SSHDisconnect
	return nil
}
