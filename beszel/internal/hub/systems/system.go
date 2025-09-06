package systems

import (
	"beszel"
	"beszel/internal/entities/system"
	"beszel/internal/hub/ws"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

type System struct {
	Id           string               `db:"id"`
	Host         string               `db:"host"`
	Port         string               `db:"port"`
	Status       string               `db:"status"`
	manager      *SystemManager       // Manager that this system belongs to
	client       *ssh.Client          // SSH client for fetching data
	data         *system.CombinedData // system data from agent
	ctx          context.Context      // Context for stopping the updater
	cancel       context.CancelFunc   // Stops and removes system from updater
	WsConn       *ws.WsConn           // Handler for agent WebSocket connection
	agentVersion semver.Version       // Agent version
	updateTicker *time.Ticker         // Ticker for updating the system
}

func (sm *SystemManager) NewSystem(systemId string) *System {
	system := &System{
		Id:   systemId,
		data: &system.CombinedData{},
	}
	system.ctx, system.cancel = system.getContext()
	return system
}

// StartUpdater starts the system updater.
// It first fetches the data from the agent then updates the records.
// If the data is not found or the system is down, it sets the system down.
func (sys *System) StartUpdater() {
	// Channel that can be used to set the system down. Currently only used to
	// allow a short delay for reconnection after websocket connection is closed.
	var downChan chan struct{}

	// Add random jitter to first WebSocket connection to prevent
	// clustering if all agents are started at the same time.
	// SSH connections during hub startup are already staggered.
	var jitter <-chan time.Time
	if sys.WsConn != nil {
		jitter = getJitter()
		// use the websocket connection's down channel to set the system down
		downChan = sys.WsConn.DownChan
	} else {
		// if the system does not have a websocket connection, wait before updating
		// to allow the agent to connect via websocket (makes sure fingerprint is set).
		time.Sleep(11 * time.Second)
	}

	// update immediately if system is not paused (only for ws connections)
	// we'll wait a minute before connecting via SSH to prioritize ws connections
	if sys.Status != paused && sys.ctx.Err() == nil {
		if err := sys.update(); err != nil {
			_ = sys.setDown(err)
		}
	}

	sys.updateTicker = time.NewTicker(time.Duration(interval) * time.Millisecond)
	// Go 1.23+ will automatically stop the ticker when the system is garbage collected, however we seem to need this or testing/synctest will block even if calling runtime.GC()
	defer sys.updateTicker.Stop()

	for {
		select {
		case <-sys.ctx.Done():
			return
		case <-sys.updateTicker.C:
			if err := sys.update(); err != nil {
				_ = sys.setDown(err)
			}
		case <-downChan:
			sys.WsConn = nil
			downChan = nil
			_ = sys.setDown(nil)
		case <-jitter:
			sys.updateTicker.Reset(time.Duration(interval) * time.Millisecond)
			if err := sys.update(); err != nil {
				_ = sys.setDown(err)
			}
		}
	}
}

// update updates the system data and records.
func (sys *System) update() error {
	if sys.Status == paused {
		sys.handlePaused()
		return nil
	}
	data, err := sys.fetchDataFromAgent()
	if err == nil {
		_, err = sys.createRecords(data)
	}
	return err
}

func (sys *System) handlePaused() {
	if sys.WsConn == nil {
		// if the system is paused and there's no websocket connection, remove the system
		_ = sys.manager.RemoveSystem(sys.Id)
	} else {
		// Send a ping to the agent to keep the connection alive if the system is paused
		if err := sys.WsConn.Ping(); err != nil {
			sys.manager.hub.Logger().Warn("Failed to ping agent", "system", sys.Id, "err", err)
			_ = sys.manager.RemoveSystem(sys.Id)
		}
	}
}

// createRecords updates the system record and adds system_stats and container_stats records
func (sys *System) createRecords(data *system.CombinedData) (*core.Record, error) {
	sys.manager.hub.Logger().Debug("Creating records - CPU array", "cpus", data.Info.Cpus)
	sys.manager.hub.Logger().Debug("Creating records - Memory array", "memory", data.Info.Memory)
	
	systemRecord, err := sys.getRecord()
	if err != nil {
		return nil, err
	}
	hub := sys.manager.hub
	// add system_stats and container_stats records
	systemStatsCollection, err := hub.FindCachedCollectionByNameOrId("system_stats")
	if err != nil {
		return nil, err
	}

	systemStatsRecord := core.NewRecord(systemStatsCollection)
	systemStatsRecord.Set("system", systemRecord.Id)
	systemStatsRecord.Set("stats", data.Stats)
	systemStatsRecord.Set("type", "1m")
	if err := hub.SaveNoValidate(systemStatsRecord); err != nil {
		return nil, err
	}
	// add new container_stats record
	if len(data.Containers) > 0 {
		containerStatsCollection, err := hub.FindCachedCollectionByNameOrId("container_stats")
		if err != nil {
			return nil, err
		}
		containerStatsRecord := core.NewRecord(containerStatsCollection)
		containerStatsRecord.Set("system", systemRecord.Id)
		containerStatsRecord.Set("stats", data.Containers)
		containerStatsRecord.Set("type", "1m")
		if err := hub.SaveNoValidate(containerStatsRecord); err != nil {
			return nil, err
		}
	}
	// update system record (do this last because it triggers alerts and we need above records to be inserted first)
	systemRecord.Set("status", up)

	systemRecord.Set("info", data.Info)
	if err := hub.SaveNoValidate(systemRecord); err != nil {
		return nil, err
	}
	return systemRecord, nil
}

// getRecord retrieves the system record from the database.
// If the record is not found, it removes the system from the manager.
func (sys *System) getRecord() (*core.Record, error) {
	record, err := sys.manager.hub.FindRecordById("systems", sys.Id)
	if err != nil || record == nil {
		_ = sys.manager.RemoveSystem(sys.Id)
		return nil, err
	}
	return record, nil
}

// setDown marks a system as down in the database.
// It takes the original error that caused the system to go down and returns any error
// encountered during the process of updating the system status.
func (sys *System) setDown(originalError error) error {
	if sys.Status == down || sys.Status == paused {
		return nil
	}
	record, err := sys.getRecord()
	if err != nil {
		return err
	}
	if originalError != nil {
		sys.manager.hub.Logger().Error("System down", "system", record.GetString("name"), "err", originalError)
	}
	record.Set("status", down)
	return sys.manager.hub.SaveNoValidate(record)
}

func (sys *System) getContext() (context.Context, context.CancelFunc) {
	if sys.ctx == nil {
		sys.ctx, sys.cancel = context.WithCancel(context.Background())
	}
	return sys.ctx, sys.cancel
}

// fetchDataFromAgent attempts to fetch data from the agent,
// prioritizing WebSocket if available.
func (sys *System) fetchDataFromAgent() (*system.CombinedData, error) {
	if sys.data == nil {
		sys.data = &system.CombinedData{}
	}

	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		wsData, err := sys.fetchDataViaWebSocket()
		if err == nil {
			return wsData, nil
		}
		// close the WebSocket connection if error and try SSH
		sys.closeWebSocketConnection()
	}

	sshData, err := sys.fetchDataViaSSH()
	if err != nil {
		return nil, err
	}
	return sshData, nil
}

func (sys *System) fetchDataViaWebSocket() (*system.CombinedData, error) {
	if sys.WsConn == nil || !sys.WsConn.IsConnected() {
		return nil, errors.New("no websocket connection")
	}
	err := sys.WsConn.RequestSystemData(sys.data)
	if err != nil {
		return nil, err
	}
	return sys.data, nil
}

// fetchDataViaSSH handles fetching data using SSH.
// This function encapsulates the original SSH logic.
// It updates sys.data directly upon successful fetch.
func (sys *System) fetchDataViaSSH() (*system.CombinedData, error) {
	maxRetries := 1
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if sys.client == nil || sys.Status == down {
			if err := sys.createSSHClient(); err != nil {
				return nil, err
			}
		}

		session, err := sys.createSessionWithTimeout(4 * time.Second)
		if err != nil {
			if attempt >= maxRetries {
				return nil, err
			}
			sys.manager.hub.Logger().Warn("Session closed. Retrying...", "host", sys.Host, "port", sys.Port, "err", err)
			sys.closeSSHConnection()
			// Reset format detection on connection failure - agent might have been upgraded
			continue
		}
		defer session.Close()

		stdout, err := session.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := session.Shell(); err != nil {
			return nil, err
		}

		*sys.data = system.CombinedData{}

		if sys.agentVersion.GTE(beszel.MinVersionCbor) {
			err = cbor.NewDecoder(stdout).Decode(sys.data)
		} else {
			err = json.NewDecoder(stdout).Decode(sys.data)
		}

		if err != nil {
			sys.closeSSHConnection()
			if attempt < maxRetries {
				continue
			}
			return nil, err
		}

		// wait for the session to complete
		if err := session.Wait(); err != nil {
			return nil, err
		}

		return sys.data, nil
	}

	// this should never be reached due to the return in the loop
	return nil, fmt.Errorf("failed to fetch data")
}

// createSSHClient creates a new SSH client for the system
func (s *System) createSSHClient() error {
	if s.manager.sshConfig == nil {
		if err := s.manager.createSSHClientConfig(); err != nil {
			return err
		}
	}
	network := "tcp"
	host := s.Host
	if strings.HasPrefix(host, "/") {
		network = "unix"
	} else {
		host = net.JoinHostPort(host, s.Port)
	}
	var err error
	s.client, err = ssh.Dial(network, host, s.manager.sshConfig)
	if err != nil {
		return err
	}
	s.agentVersion, _ = extractAgentVersion(string(s.client.Conn.ServerVersion()))
	return nil
}

// createSessionWithTimeout creates a new SSH session with a timeout to avoid hanging
// in case of network issues
func (sys *System) createSessionWithTimeout(timeout time.Duration) (*ssh.Session, error) {
	if sys.client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	ctx, cancel := context.WithTimeout(sys.ctx, timeout)
	defer cancel()

	sessionChan := make(chan *ssh.Session, 1)
	errChan := make(chan error, 1)

	go func() {
		if session, err := sys.client.NewSession(); err != nil {
			errChan <- err
		} else {
			sessionChan <- session
		}
	}()

	select {
	case session := <-sessionChan:
		return session, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout")
	}
}

// closeSSHConnection closes the SSH connection but keeps the system in the manager
func (sys *System) closeSSHConnection() {
	if sys.client != nil {
		sys.client.Close()
		sys.client = nil
	}
}

// closeWebSocketConnection closes the WebSocket connection but keeps the system in the manager
// to allow updating via SSH. It will be removed if the WS connection is re-established.
// The system will be set as down a few seconds later if the connection is not re-established.
func (sys *System) closeWebSocketConnection() {
	if sys.WsConn != nil {
		sys.WsConn.Close(nil)
	}
}

// extractAgentVersion extracts the beszel version from SSH server version string
func extractAgentVersion(versionString string) (semver.Version, error) {
	_, after, _ := strings.Cut(versionString, "_")
	return semver.Parse(after)
}

// getJitter returns a channel that will be triggered after a random delay
// between 40% and 90% of the interval.
// This is used to stagger the initial WebSocket connections to prevent clustering.
func getJitter() <-chan time.Time {
	minPercent := 40
	maxPercent := 90
	jitterRange := maxPercent - minPercent
	msDelay := (interval * minPercent / 100) + rand.Intn(interval*jitterRange/100)
	return time.After(time.Duration(msDelay) * time.Millisecond)
}
