package hub

import (
	"beszel"
	"beszel/internal/entities/system"
	"database/sql"
	"net"
	"strings"

	"github.com/goccy/go-json"
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

	hostname := strings.Join(parts[:len(parts)-1], "@")
	connectionKey := parts[len(parts)-1]

	if connectionKey == "" {
		h.Logger().Debug("new system did not supply an connection key")
		return
	}

	validKey, user := h.checkConnectionKey(connectionKey)
	if !validKey {
		h.Logger().Info("new system did not supply valid connection key")
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
		if err != sql.ErrNoRows {
			h.Logger().Error("Failed to fetch client record", "err", err)
			return
		}
	}

	// If this client doesnt already exist in the systems collection
	if err == sql.ErrNoRows {
		h.Logger().Info("Unknown client tried to connect", "fingerprint", fingerprint, "address", c.RemoteAddr())

		// check to see if it already has a new_systems entry to display it
		_, err = h.FindFirstRecordByFilter(
			"new_systems",
			"fingerprint={:fingerprint}",
			dbx.Params{"fingerprint": fingerprint},
		)

		if err != nil {
			if err != sql.ErrNoRows {
				h.Logger().Error("failed to get pending system records", "err", err)
				return
			}
			// If it has no entry already, determine what to do with it

			switch withApiAction {
			case "accept":
				r, err := h.acceptSystem(user, hostname, c.RemoteAddr().String(), fingerprint)
				if err != nil {
					h.Logger().Error("failed to accept new connection", "err", err)
					return
				}

				record = r
				// accept the system and allow it to drop through to start recording details immediately
			case "deny":
				// do not add entry to new_systems collection when denying
				return
			default:

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
				newSystemRecord.Set("hostname", hostname)
				newSystemRecord.Set("fingerprint", fingerprint)
				newSystemRecord.Set("address", address)

				err = h.Save(newSystemRecord)
				if err != nil {
					h.Logger().Error("failed to save pending system record", "err", err)
					return
				}

				return
			}
		}
		// intentional fallthrough
	}

	if _, ok := h.Store().GetOk(record.Id); ok {
		h.Logger().Warn("Client with same fingerprint attempted to connect", "fingerprint", fingerprint, "address", c.RemoteAddr())

		return
	}
	h.Store().Set(record.Id, sshConn)

	defer func() {
		// When channel := range chans finishes this indicates the client has disconnected so remove the client connection lock
		h.Store().Remove(record.Id)
		h.updateSystemStatus(record, "down")
	}()

	for channel := range chans {
		if channel.ChannelType() != "stats" {
			channel.Reject(ssh.Prohibited, "Invalid channel type")
			h.Logger().Warn("client tried to open an invalid channel", "type", channel.ChannelType(), "address", c.RemoteAddr())
			return
		}

		if record.GetString("status") == "paused" {
			return
		}

		channel, reqs, err := channel.Accept()
		if err != nil {
			h.Logger().Warn("failed to accept client channe", "err", err)
			return
		}
		go ssh.DiscardRequests(reqs)

		var systemData system.CombinedData
		if err := json.NewDecoder(channel).Decode(&systemData); err != nil {
			h.updateSystemStatus(record, "down")
			return
		}

		h.updateSystemRecord(record, systemData)
		channel.Close()
	}
}

func (h *Hub) acceptSystem(user any, name, address, fingerprint string) (*core.Record, error) {

	collection, err := h.FindCollectionByNameOrId("systems")
	if err != nil {
		h.Logger().Error("failed to get systems collection", "err", err)
		return nil, err
	}

	newSystemRecord := core.NewRecord(collection)
	newSystemRecord.Set("status", "pending")
	newSystemRecord.Set("name", name)
	newSystemRecord.Set("host", address)
	newSystemRecord.Set("port", "N/A")
	newSystemRecord.Set("type", "client")
	newSystemRecord.Set("fingerprint", fingerprint)
	newSystemRecord.Set("users", user)

	err = h.Save(newSystemRecord)
	if err != nil {
		h.Logger().Error("failed to save pending system record", "err", err)
		return nil, err
	}
	return newSystemRecord, nil
}

func (h *Hub) checkConnectionKey(key string) (bool, any) {
	if key == "" {
		return false, nil
	}
	// get user settings
	record, err := h.FindFirstRecordByFilter(
		"user_settings", "connection_key={:key}",
		dbx.Params{"key": key},
	)
	if err != nil {
		h.Logger().Error("Failed to get user settings", "err", err.Error())
		return false, nil
	}

	return record.GetString("connection_key") == key, record.Get("user")
}
