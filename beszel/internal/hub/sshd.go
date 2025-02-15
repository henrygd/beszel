package hub

import (
	"beszel"
	"database/sql"
	"net"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

func (h *Hub) startSSHServer(addr string) error {
	privateKey, err := h.getSSHKey()
	if err != nil {
		h.Logger().Error("Failed to get SSH key: ", "err", err.Error())
		return err
	}

	config := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-" + beszel.AppName + "-" + beszel.Version,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{
				Extensions: map[string]string{
					"fingerprint": ssh.FingerprintSHA256(key),
				},
			}, nil
		},
	}
	config.AddHostKey(privateKey)

	sshListener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := sshListener.Accept()
			if err != nil {
				h.Logger().Warn("Failed to accept incoming connection", "err", err)
				continue
			}

			go h.acceptConn(conn, config)
		}
	}()

	return nil
}

func (h *Hub) acceptConn(c net.Conn, config *ssh.ServerConfig) {

	h.Logger().Info("new system connected", "address", c.RemoteAddr())
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(c, config)
	if err != nil {
		h.Logger().Info("Failed to SSH handshake", "err", err)
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	parts := strings.Split(sshConn.User(), "@")
	if len(parts) < 2 {
		h.Logger().Debug("new system did not supply valid user string")
		return
	}

	Hostname := strings.Join(parts[:len(parts)-1], "@")
	ApiKey := parts[len(parts)-1]

	if ApiKey == "" {
		h.Logger().Debug("new system did not supply an api key")
		return
	}

	if !h.checkAPIKey(ApiKey) {
		h.Logger().Debug("new system did not supply valid api key")
		return
	}

	settings, err := h.FindFirstRecordByFilter("connection_settings", "")
	if err != nil {
		h.Logger().Error("Unable to get connection settings", "err", err)
		return
	}

	withApiAction := settings.GetString("withAPIKey")

	fingerprint := sshConn.Permissions.Extensions["fingerprint"]

	_, err = h.FindFirstRecordByData("blocked_systems", "fingerprint", fingerprint)
	// If we could find a record for this sytem in the blocked_systems table, then early quit
	if err == nil {
		h.Logger().Debug("Blocked system attempted to connect", "fingerprint", fingerprint, "address", c.RemoteAddr())
		return
	} else {

		if err != sql.ErrNoRows {
			h.Logger().Warn("Could not read blocked_systems table", "err", err, "fingerprint", fingerprint)
			return
		}

		// If the error was that there were no matching rows then pass it
		h.Logger().Info("System is not blocked!", "fingerprint", fingerprint)
	}

	record, err := h.FindFirstRecordByFilter(
		"2hz5ncl8tizk5nx", // systems collection
		"status!='paused' && fingerprint={:fingerprint}",
		dbx.Params{"fingerprint": fingerprint},
	)
	if err != nil {
		if err == sql.ErrNoRows {
			h.Logger().Info("Unknown client tried to connect", "fingerprint", fingerprint, "address", c.RemoteAddr())

			_, err = h.FindFirstRecordByFilter(
				"new_systems",
				"fingerprint={:fingerprint}",
				dbx.Params{"fingerprint": fingerprint},
			)

			if err != nil {
				if err == sql.ErrNoRows {

					switch withApiAction {
					case "accept":
						h.acceptSystem(fingerprint)
						return
					case "deny":
						// do not add entry to new_systems collection when denying
						return
					default:
						// intentional fallthrough for "display"
					}

					collection, err := h.FindCollectionByNameOrId("new_systems")
					if err != nil {
						h.Logger().Error("failed to get new_systems collection", "err", err)
						return
					}

					address, _, err := net.SplitHostPort(c.RemoteAddr().String())
					if err != nil {
						h.Logger().Debug("Could not split remote address host and port", "address", c.RemoteAddr().String(), "err", err)
						address = c.RemoteAddr().String()
					}

					newSystemRecord := core.NewRecord(collection)
					newSystemRecord.Set("hostname", Hostname)
					newSystemRecord.Set("fingerprint", fingerprint)
					newSystemRecord.Set("address", address)

					err = h.Save(newSystemRecord)
					if err != nil {
						h.Logger().Error("failed to save pending system record", "err", err)
						return
					}
				} else {
					h.Logger().Error("failed to get pending system records", "err", err)
					return
				}
			}

			return
		}
		h.Logger().Error("Failed to fetch client record", "err", err)
		return
	}

	if _, ok := h.Store().GetOk(record.Id); ok {
		h.Logger().Warn("Client with same fingerprint attempted to connect", "fingerprint", fingerprint, "address", c.RemoteAddr())
		return
	}
	h.Store().Set(record.Id, sshConn)

	defer func() {
		// When channel := range chans finishes this indicates the client has disconnected so remove the client connection lock
		h.Store().Remove(record.Id)
	}()

	for channel := range chans {
		if channel.ChannelType() != "stats" {
			channel.Reject(ssh.Prohibited, "Invalid channel type")
			h.Logger().Warn("client tried to open an invalid channel", "type", channel.ChannelType(), "address", c.RemoteAddr())
			return
		}

	}
}

func (h *Hub) acceptSystem(fingerprint string) error {

	collection, err := h.FindCollectionByNameOrId("blocked_systems")
	if err != nil {
		h.Logger().Error("failed to get blocked_systems collection", "err", err)
		return err
	}

	newSystemBlockedRecord := core.NewRecord(collection)
	newSystemBlockedRecord.Set("fingerprint", fingerprint)

	err = h.Save(newSystemBlockedRecord)
	if err != nil {
		h.Logger().Error("failed to save pending system record", "err", err)
		return err
	}
	return nil
}

func (h *Hub) checkAPIKey(key string) bool {
	// get user settings
	record, err := h.FindFirstRecordByFilter(
		"user_settings", "api_key={:key}",
		dbx.Params{"key": key},
	)
	if err != nil {
		h.Logger().Error("Failed to get user settings", "err", err.Error())
		return false
	}

	return record.GetString("api_key") != key
}
