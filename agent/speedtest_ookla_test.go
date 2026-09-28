//go:build testing

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real Ookla CLI 1.2.0 output, with interface addresses anonymized.
const ooklaResultJSON = `{"type":"result","timestamp":"2026-09-26T16:51:54Z","ping":{"jitter":0.159,"latency":4.050,"low":3.871,"high":4.226},"download":{"bandwidth":129197602,"bytes":915984428,"elapsed":7213,"latency":{"iqm":21.424,"low":3.984,"high":48.748,"jitter":2.169}},"upload":{"bandwidth":127497495,"bytes":458073236,"elapsed":3600,"latency":{"iqm":3.676,"low":3.295,"high":6.417,"jitter":0.297}},"packetLoss":0,"isp":"Odido Netherlands","interface":{"internalIp":"192.168.1.2","name":"eth0","macAddr":"00:00:00:00:00:00","isVpn":false,"externalIp":"203.0.113.1"},"server":{"id":52365,"host":"speedtest.ams.t-mobile.nl","port":8080,"name":"Odido","location":"Amsterdam","country":"Netherlands","ip":"37.143.86.95"},"result":{"id":"f7a9c6df-f1a0-46a2-be24-800e575ae9b7","url":"https://www.speedtest.net/result/c/f7a9c6df-f1a0-46a2-be24-800e575ae9b7","persisted":true}}`

func TestParseOoklaOutput(t *testing.T) {
	result, err := parseOoklaOutput([]byte("\n" + ooklaResultJSON + "\n"))
	require.NoError(t, err)
	assert.Equal(t, speedtest.Result{
		Download:        129197602,
		Upload:          127497495,
		Ping:            4.05,
		Jitter:          0.159,
		DownloadLatency: speedtest.Latency{IQM: 21.424, Jitter: 2.169},
		UploadLatency:   speedtest.Latency{IQM: 3.676, Jitter: 0.297},
		Loss:            0,
		ServerID:        52365,
		ServerName:      "Odido",
		ServerLocation:  "Amsterdam, Netherlands",
		ISP:             "Odido Netherlands",
		URL:             "https://www.speedtest.net/result/c/f7a9c6df-f1a0-46a2-be24-800e575ae9b7",
	}, result)
}

func TestParseOoklaOutputWithoutPacketLoss(t *testing.T) {
	result, err := parseOoklaOutput([]byte(`{"type":"result","download":{"bandwidth":1},"upload":{"bandwidth":2},"server":{"id":1,"location":"Paris"}}`))
	require.NoError(t, err)
	assert.Equal(t, -1.0, result.Loss)
	assert.Equal(t, "Paris", result.ServerLocation)
}

func TestParseOoklaOutputInvalid(t *testing.T) {
	for _, output := range []string{
		"",
		// Output of the unrelated Python speedtest-cli, which is also named `speedtest`.
		"Retrieving speedtest.net configuration...\nDownload: 93.12 Mbit/s",
		`{"type":"log","level":"info","message":"hello"}`,
	} {
		_, err := parseOoklaOutput([]byte(output))
		assert.Error(t, err, output)
	}
}

func TestOoklaErrorMessage(t *testing.T) {
	// Real output for an unknown --server-id.
	assert.Equal(t, "Configuration - No servers defined (NoServersException)", ooklaErrorMessage([]byte(
		`{"type":"log","timestamp":"2026-09-26T16:58:14Z","message":"Configuration - No servers defined (NoServersException)","level":"error"}`+"\n",
	)))
	assert.Equal(t, "No servers defined (NoServersException)", ooklaErrorMessage([]byte(
		"[2026-09-26 10:00:00.000] [error] something\n[error] No servers defined (NoServersException)\n",
	)))
	assert.Empty(t, ooklaErrorMessage([]byte(`{"type":"log","level":"warning","message":"slow"}`)))
}
