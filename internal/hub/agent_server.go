package hub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/hub/utils"

	"github.com/pocketbase/pocketbase/core"
)

const agentConnectPath = "/api/beszel/agent-connect"

// parseAgentListenAddr converts the AGENT_PORT value to a listen address.
// A bare port ("8091") listens on all interfaces; otherwise the value must be
// a host:port pair ("127.0.0.1:8091", ":8091", "[::1]:8091"). An empty value
// returns an empty address, meaning the agent server is disabled.
func parseAgentListenAddr(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	host, port := "", value
	if strings.Contains(value, ":") {
		var err error
		if host, port, err = net.SplitHostPort(value); err != nil {
			return "", fmt.Errorf("invalid AGENT_PORT %q: %w", value, err)
		}
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid AGENT_PORT %q: port must be a number between 1 and 65535", value)
	}
	return net.JoinHostPort(host, port), nil
}

// newAgentServerHandler returns a handler that only serves the agent WebSocket
// endpoint. Every other path returns 404, so the UI and API are not reachable
// through the agent server.
func (h *Hub) newAgentServerHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+agentConnectPath, func(w http.ResponseWriter, r *http.Request) {
		acr := agentConnectRequest{req: r, res: w, hub: h}
		_ = acr.agentConnect()
	})
	return mux
}

// startAgentServer starts an additional listener that only exposes the agent
// connection endpoint, if AGENT_PORT is set. This allows the main port (UI and
// API) to be kept private while agents connect through a separate port.
func (h *Hub) startAgentServer(se *core.ServeEvent) error {
	value, _ := utils.GetEnv("AGENT_PORT")
	addr, err := parseAgentListenAddr(value)
	if err != nil || addr == "" {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start agent server: %w", err)
	}
	server := &http.Server{
		Handler:           h.newAgentServerHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			se.App.Logger().Error("Agent server stopped", "err", err)
		}
	}()
	se.App.Logger().Info("Agent server started", "addr", listener.Addr().String())

	se.App.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return e.Next()
	})
	return nil
}
