package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func groupedContainers() []*container.Stats {
	return []*container.Stats{
		{Name: "mentors_web.1.kz9", Cpu: 1, Mem: 2, Stack: "mentors", Service: "web"},
		{Name: "mentors_web.2.ab3", Cpu: 3, Mem: 4, Stack: "mentors", Service: "web"},
		{Name: "solo", Cpu: 5, Mem: 6},
	}
}

func assertGroups(t *testing.T, containers []*container.Stats) {
	t.Helper()
	require.Len(t, containers, 3)
	for _, ctr := range containers[:2] {
		assert.Equal(t, "mentors", ctr.Stack)
		assert.Equal(t, "web", ctr.Service)
	}
	assert.NotEqual(t, containers[0].Name, containers[1].Name, "replicas share a group but not a name")
	assert.Empty(t, containers[2].Stack)
	assert.Empty(t, containers[2].Service)
}

// A 0.19+ agent answers GetData in the generic Data field; an older one fills SystemData. Both
// paths have to carry the container group.
func TestUnmarshalGetDataCarriesContainerGroups(t *testing.T) {
	data := &system.CombinedData{Containers: groupedContainers()}

	t.Run("generic data field", func(t *testing.T) {
		encoded, err := cbor.Marshal(data)
		require.NoError(t, err)
		var result system.CombinedData
		require.NoError(t, UnmarshalResponse(common.AgentResponse{Data: encoded}, common.GetData, &result))
		assertGroups(t, result.Containers)
	})

	t.Run("legacy typed field", func(t *testing.T) {
		var result system.CombinedData
		require.NoError(t, UnmarshalResponse(common.AgentResponse{SystemData: data}, common.GetData, &result))
		assertGroups(t, result.Containers)
	})
}

// newSSHStatsServer starts a loopback SSH server that answers one request with response and
// returns a transport pointed at it. The helper in ssh_test.go is built for stalling scenarios
// and always answers with a control response, so it cannot carry stats.
func newSSHStatsServer(t *testing.T, response common.AgentResponse) *SSHTransport {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &ssh.ServerConfig{NoClientAuth: true, ServerVersion: "SSH-2.0-beszel_0.21.1"}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	transport := NewSSHTransport(SSHTransportConfig{
		Host: host, Port: port, Timeout: 5 * time.Second,
		Config: &ssh.ClientConfig{
			User:            "test",
			HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
			Timeout:         2 * time.Second,
		},
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveStatsRequest(conn, config, response)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		transport.Close()
	})
	return transport
}

func serveStatsRequest(conn net.Conn, config *ssh.ServerConfig, response common.AgentResponse) {
	defer func() { _ = conn.Close() }()
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer func() { _ = server.Close() }()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		ch, chRequests, err := channel.Accept()
		if err != nil {
			return
		}
		for request := range chRequests {
			if request.Type != "shell" {
				_ = request.Reply(false, nil)
				continue
			}
			_ = request.Reply(true, nil)
			var req common.HubRequest[cbor.RawMessage]
			if cbor.NewDecoder(ch).Decode(&req) != nil {
				_ = ch.Close()
				return
			}
			if cbor.NewEncoder(ch).Encode(response) != nil {
				_ = ch.Close()
				return
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			_ = ch.Close()
			return
		}
	}
}

// The SSH fallback encodes the same AgentResponse with the same codec as the WebSocket path, but
// the hub decodes it from a session pipe, so the whole way through is worth one test.
func TestSSHRequestCarriesContainerGroups(t *testing.T) {
	transport := newSSHStatsServer(t, common.AgentResponse{
		SystemData: &system.CombinedData{Containers: groupedContainers()},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var data system.CombinedData
	require.NoError(t, transport.Request(ctx, common.GetData, nil, &data))
	assertGroups(t, data.Containers)
}

// An agent that predates the fields sends a response without cbor keys 12 and 13; the hub reads
// the containers and leaves the group empty.
func TestSSHRequestFromAgentWithoutGroups(t *testing.T) {
	transport := newSSHStatsServer(t, common.AgentResponse{
		SystemData: &system.CombinedData{Containers: []*container.Stats{
			{Name: "solo", Cpu: 5, Mem: 6},
		}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var data system.CombinedData
	require.NoError(t, transport.Request(ctx, common.GetData, nil, &data))
	require.Len(t, data.Containers, 1)
	assert.Equal(t, "solo", data.Containers[0].Name)
	assert.Empty(t, data.Containers[0].Stack)
	assert.Empty(t, data.Containers[0].Service)
}
