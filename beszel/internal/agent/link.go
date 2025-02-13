package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	// We are using this for random stats collection timing
	insecureRandom "math/rand"

	sshServer "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

func (a *Agent) writeStats(output io.Writer) (system.CombinedData, error) {
	stats := a.gatherStats()
	return stats, json.NewEncoder(output).Encode(stats)
}

func (a *Agent) startServer(pubKey []byte, addr string) {

	sshServer.Handle(func(s sshServer.Session) {
		if stats, err := a.writeStats(s); err != nil {
			slog.Error("Error encoding stats", "err", err, "stats", stats)
			s.Exit(1)
			return
		}

		s.Exit(0)
	})

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

func (a *Agent) startClient(pubKey []byte, addr string) {

	slog.Info("Connecting to beszel SSH server", "address", addr)
	signer, err := createOrLoadKey("id_ed25519")
	if err != nil {
		slog.Error("Failed to load private key: ", "err", err)
		os.Exit(1)
	}

	allowed, _, _, _, err := sshServer.ParseAuthorizedKey(pubKey)
	if err != nil {
		slog.Error("Failed to parse server public key: ", "err", err)
		os.Exit(1)
	}

	c := &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(allowed),
		ClientVersion:   beszel.AppName + "-" + beszel.Version,
		Timeout:         10 * time.Second,
	}

	lastBackoff := 1
	backoff := func() {
		currentBackoff := 2 * time.Second * time.Duration(lastBackoff)
		slog.Info("Waiting to reconnect: ", "backoff", currentBackoff)
		time.Sleep(currentBackoff)
		lastBackoff++
	}

	for {

		conn, err := net.DialTimeout("tcp", addr, c.Timeout)
		if err != nil {
			slog.Error("Failed to connect to server: ", "addr", addr, "err", err)
			backoff()
			continue
		}

		clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, c)
		if err != nil {
			conn.Close()
			slog.Error("Failed to join to server: ", "addr", addr, "err", err)
			backoff()
			continue
		}

		go ssh.DiscardRequests(reqs)
		go a.discardChannels(chans)

		for {

			dataChan, req, err := clientConn.OpenChannel(beszel.AppName+"-"+beszel.Version+"-stats", nil)
			if err != nil {
				slog.Warn("failed to send statistics, server did not allow opening of stats channel", "err", err)
				a.randomSleep(15, 30)
				continue
			}

			go ssh.DiscardRequests(req)

			if stats, err := a.writeStats(dataChan); err != nil {
				slog.Error("Error writing stats", "err", err, "stats", stats)
				a.exit(dataChan, 1)
				return
			}

			a.exit(dataChan, 0)

			// Make sure that all clients arent syncing up and sending massive amounts of data at once
			a.randomSleep(15, 30)
		}

	}

}

func (a *Agent) exit(channel ssh.Channel, code int) error {
	status := struct{ Status uint32 }{uint32(code)}
	_, err := channel.SendRequest("exit-status", false, ssh.Marshal(&status))
	if err != nil {
		return err
	}
	return channel.Close()
}

func (a *Agent) discardChannels(chans <-chan ssh.NewChannel) {
	for c := range chans {
		c.Reject(ssh.Prohibited, "Clients do not support connections")
	}
}

func (a *Agent) randomSleep(min, max int) {
	variance := insecureRandom.Intn(min)
	time.Sleep(time.Duration(variance)*time.Second + time.Duration(min))
}

// https://github.com/NHAS/reverse_ssh/blob/main/internal/server/server.go
func createOrLoadKey(privateKeyPath string) (ssh.Signer, error) {

	//If we have already created a private key (or there is one in the current directory) dont overwrite/create another one
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {

		privateKeyPem, err := generatePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("unable to generate private key, and no private key specified: %s", err)
		}

		err = os.WriteFile(privateKeyPath, privateKeyPem, 0600)
		if err != nil {
			return nil, fmt.Errorf("unable to write private key to disk: %s", err)
		}
	}

	privateBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key (%s): %s", privateKeyPath, err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %s", err)
	}

	return private, nil
}

// https://github.com/NHAS/reverse_ssh/blob/71420af670aebbe632f35ce8428cbfbd21dc5f53/internal/global.go#L44
func generatePrivateKey() ([]byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Convert a generated ed25519 key into a PEM block so that the ssh library can ingest it, bit round about tbh
	bytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}

	privatePem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: bytes,
		},
	)

	return privatePem, nil
}
