package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"

	"github.com/henrygd/beszel"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/pocketbase/dbx"
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
	smartOnce    sync.Once            // Once for fetching and saving smart devices
	detailsOnce  sync.Once            // Once for fetching and saving static system details
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
	options := common.DataRequestOptions{
		CacheTimeMs: uint16(interval),
	}
	// fetch system details only on the first update
	sys.detailsOnce.Do(func() {
		options.IncludeDetails = true
	})
	data, err := sys.fetchDataFromAgent(options)
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
	systemRecord, err := sys.getRecord()
	if err != nil {
		return nil, err
	}
	hub := sys.manager.hub
	err = hub.RunInTransaction(func(txApp core.App) error {
		if data.Details != nil {
			slog.Info("Static info", "data", data.Details)
			if err := createStaticInfoRecord(txApp, data.Details, sys.Id); err != nil {
				return err
			}
		}
		// add system_stats and container_stats records
		systemStatsCollection, err := txApp.FindCachedCollectionByNameOrId("system_stats")
		if err != nil {
			return err
		}

		systemStatsRecord := core.NewRecord(systemStatsCollection)
		systemStatsRecord.Set("system", systemRecord.Id)
		systemStatsRecord.Set("stats", data.Stats)
		systemStatsRecord.Set("type", "1m")
		if err := txApp.SaveNoValidate(systemStatsRecord); err != nil {
			return err
		}
		if len(data.Containers) > 0 {
			// add / update containers records
			if data.Containers[0].Id != "" {
				if err := createContainerRecords(txApp, data.Containers, sys.Id); err != nil {
					return err
				}
			}
			// add new container_stats record
			containerStatsCollection, err := txApp.FindCachedCollectionByNameOrId("container_stats")
			if err != nil {
				return err
			}
			containerStatsRecord := core.NewRecord(containerStatsCollection)
			containerStatsRecord.Set("system", systemRecord.Id)
			containerStatsRecord.Set("stats", data.Containers)
			containerStatsRecord.Set("type", "1m")
			if err := txApp.SaveNoValidate(containerStatsRecord); err != nil {
				return err
			}
		}

		// add new systemd_stats record
		if len(data.SystemdServices) > 0 {
			if err := createSystemdStatsRecords(txApp, data.SystemdServices, sys.Id); err != nil {
				return err
			}
		}

		// update system record (do this last because it triggers alerts and we need above records to be inserted first)
		systemRecord.Set("status", up)

		systemRecord.Set("info", data.Info)
		if err := txApp.SaveNoValidate(systemRecord); err != nil {
			return err
		}
		return nil
	})

	// Fetch and save SMART devices when system first comes online
	if err == nil {
		sys.smartOnce.Do(func() {
			go sys.FetchAndSaveSmartDevices()
		})
	}

	return systemRecord, err
}

func createStaticInfoRecord(app core.App, data *system.Details, systemId string) error {
	record, err := app.FindRecordById("system_details", systemId)
	if err != nil {
		collection, err := app.FindCollectionByNameOrId("system_details")
		if err != nil {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("id", systemId)
	}
	record.Set("system", systemId)
	record.Set("hostname", data.Hostname)
	record.Set("kernel", data.Kernel)
	record.Set("cores", data.Cores)
	record.Set("threads", data.Threads)
	record.Set("cpu", data.CpuModel)
	record.Set("os", data.Os)
	record.Set("os_name", data.OsName)
	record.Set("arch", data.Arch)
	record.Set("memory", data.MemoryTotal)
	record.Set("podman", data.Podman)
	return app.SaveNoValidate(record)
}

func createSystemdStatsRecords(app core.App, data []*systemd.Service, systemId string) error {
	if len(data) == 0 {
		return nil
	}
	// shared params for all records
	params := dbx.Params{
		"system":  systemId,
		"updated": time.Now().UTC().UnixMilli(),
	}

	valueStrings := make([]string, 0, len(data))
	for i, service := range data {
		suffix := fmt.Sprintf("%d", i)
		valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:state%[1]s}, {:sub%[1]s}, {:cpu%[1]s}, {:cpuPeak%[1]s}, {:memory%[1]s}, {:memPeak%[1]s}, {:updated})", suffix))
		params["id"+suffix] = makeStableHashId(systemId, service.Name)
		params["name"+suffix] = service.Name
		params["state"+suffix] = service.State
		params["sub"+suffix] = service.Sub
		params["cpu"+suffix] = service.Cpu
		params["cpuPeak"+suffix] = service.CpuPeak
		params["memory"+suffix] = service.Mem
		params["memPeak"+suffix] = service.MemPeak
	}
	queryString := fmt.Sprintf(
		"INSERT INTO systemd_services (id, system, name, state, sub, cpu, cpuPeak, memory, memPeak, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system = excluded.system, name = excluded.name, state = excluded.state, sub = excluded.sub, cpu = excluded.cpu, cpuPeak = excluded.cpuPeak, memory = excluded.memory, memPeak = excluded.memPeak, updated = excluded.updated",
		strings.Join(valueStrings, ","),
	)
	_, err := app.DB().NewQuery(queryString).Bind(params).Execute()
	return err
}

// createContainerRecords creates container records
func createContainerRecords(app core.App, data []*container.Stats, systemId string) error {
	if len(data) == 0 {
		return nil
	}
	// shared params for all records
	params := dbx.Params{
		"system":  systemId,
		"updated": time.Now().UTC().UnixMilli(),
	}
	valueStrings := make([]string, 0, len(data))
	for i, container := range data {
		suffix := fmt.Sprintf("%d", i)
		valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:image%[1]s}, {:status%[1]s}, {:health%[1]s}, {:cpu%[1]s}, {:memory%[1]s}, {:net%[1]s}, {:updated})", suffix))
		params["id"+suffix] = container.Id
		params["name"+suffix] = container.Name
		params["image"+suffix] = container.Image
		params["status"+suffix] = container.Status
		params["health"+suffix] = container.Health
		params["cpu"+suffix] = container.Cpu
		params["memory"+suffix] = container.Mem
		params["net"+suffix] = container.NetworkSent + container.NetworkRecv
	}
	queryString := fmt.Sprintf(
		"INSERT INTO containers (id, system, name, image, status, health, cpu, memory, net, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system = excluded.system, name = excluded.name, image = excluded.image, status = excluded.status, health = excluded.health, cpu = excluded.cpu, memory = excluded.memory, net = excluded.net, updated = excluded.updated",
		strings.Join(valueStrings, ","),
	)
	_, err := app.DB().NewQuery(queryString).Bind(params).Execute()
	return err
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
func (sys *System) fetchDataFromAgent(options common.DataRequestOptions) (*system.CombinedData, error) {
	if sys.data == nil {
		sys.data = &system.CombinedData{}
	}

	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		wsData, err := sys.fetchDataViaWebSocket(options)
		if err == nil {
			return wsData, nil
		}
		// close the WebSocket connection if error and try SSH
		sys.closeWebSocketConnection()
	}

	sshData, err := sys.fetchDataViaSSH(options)
	if err != nil {
		return nil, err
	}
	return sshData, nil
}

func (sys *System) fetchDataViaWebSocket(options common.DataRequestOptions) (*system.CombinedData, error) {
	if sys.WsConn == nil || !sys.WsConn.IsConnected() {
		return nil, errors.New("no websocket connection")
	}
	err := sys.WsConn.RequestSystemData(context.Background(), sys.data, options)
	if err != nil {
		return nil, err
	}
	return sys.data, nil
}

// fetchStringFromAgentViaSSH is a generic function to fetch strings via SSH
func (sys *System) fetchStringFromAgentViaSSH(action common.WebSocketAction, requestData any, errorMsg string) (string, error) {
	var result string
	err := sys.runSSHOperation(4*time.Second, 1, func(session *ssh.Session) (bool, error) {
		stdout, err := session.StdoutPipe()
		if err != nil {
			return false, err
		}
		stdin, stdinErr := session.StdinPipe()
		if stdinErr != nil {
			return false, stdinErr
		}
		if err := session.Shell(); err != nil {
			return false, err
		}
		reqDataBytes, _ := cbor.Marshal(requestData)
		req := common.HubRequest[cbor.RawMessage]{Action: action, Data: reqDataBytes}
		_ = cbor.NewEncoder(stdin).Encode(req)
		_ = stdin.Close()
		var resp common.AgentResponse
		err = cbor.NewDecoder(stdout).Decode(&resp)
		if err != nil {
			return false, err
		}
		if resp.String == nil {
			return false, errors.New(errorMsg)
		}
		result = *resp.String
		return false, nil
	})
	return result, err
}

// FetchContainerInfoFromAgent fetches container info from the agent
func (sys *System) FetchContainerInfoFromAgent(containerID string) (string, error) {
	// fetch via websocket
	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return sys.WsConn.RequestContainerInfo(ctx, containerID)
	}
	// fetch via SSH
	return sys.fetchStringFromAgentViaSSH(common.GetContainerInfo, common.ContainerInfoRequest{ContainerID: containerID}, "no info in response")
}

// FetchContainerLogsFromAgent fetches container logs from the agent
func (sys *System) FetchContainerLogsFromAgent(containerID string) (string, error) {
	// fetch via websocket
	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return sys.WsConn.RequestContainerLogs(ctx, containerID)
	}
	// fetch via SSH
	return sys.fetchStringFromAgentViaSSH(common.GetContainerLogs, common.ContainerLogsRequest{ContainerID: containerID}, "no logs in response")
}

// FetchSystemdInfoFromAgent fetches detailed systemd service information from the agent
func (sys *System) FetchSystemdInfoFromAgent(serviceName string) (systemd.ServiceDetails, error) {
	// fetch via websocket
	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return sys.WsConn.RequestSystemdInfo(ctx, serviceName)
	}

	var result systemd.ServiceDetails
	err := sys.runSSHOperation(5*time.Second, 1, func(session *ssh.Session) (bool, error) {
		stdout, err := session.StdoutPipe()
		if err != nil {
			return false, err
		}
		stdin, stdinErr := session.StdinPipe()
		if stdinErr != nil {
			return false, stdinErr
		}
		if err := session.Shell(); err != nil {
			return false, err
		}

		reqDataBytes, _ := cbor.Marshal(common.SystemdInfoRequest{ServiceName: serviceName})
		req := common.HubRequest[cbor.RawMessage]{Action: common.GetSystemdInfo, Data: reqDataBytes}
		if err := cbor.NewEncoder(stdin).Encode(req); err != nil {
			return false, err
		}
		_ = stdin.Close()

		var resp common.AgentResponse
		if err := cbor.NewDecoder(stdout).Decode(&resp); err != nil {
			return false, err
		}
		if resp.ServiceInfo == nil {
			if resp.Error != "" {
				return false, errors.New(resp.Error)
			}
			return false, errors.New("no systemd info in response")
		}
		result = resp.ServiceInfo
		return false, nil
	})

	return result, err
}

func makeStableHashId(strings ...string) string {
	hash := fnv.New32a()
	for _, str := range strings {
		hash.Write([]byte(str))
	}
	return fmt.Sprintf("%x", hash.Sum32())
}

// fetchDataViaSSH handles fetching data using SSH.
// This function encapsulates the original SSH logic.
// It updates sys.data directly upon successful fetch.
func (sys *System) fetchDataViaSSH(options common.DataRequestOptions) (*system.CombinedData, error) {
	err := sys.runSSHOperation(4*time.Second, 1, func(session *ssh.Session) (bool, error) {
		stdout, err := session.StdoutPipe()
		if err != nil {
			return false, err
		}
		stdin, stdinErr := session.StdinPipe()
		if err := session.Shell(); err != nil {
			return false, err
		}

		*sys.data = system.CombinedData{}

		if sys.agentVersion.GTE(beszel.MinVersionAgentResponse) && stdinErr == nil {
			reqDataBytes, _ := cbor.Marshal(options)
			req := common.HubRequest[cbor.RawMessage]{Action: common.GetData, Data: reqDataBytes}
			_ = cbor.NewEncoder(stdin).Encode(req)
			_ = stdin.Close()

			var resp common.AgentResponse
			if decErr := cbor.NewDecoder(stdout).Decode(&resp); decErr == nil && resp.SystemData != nil {
				*sys.data = *resp.SystemData
				if err := session.Wait(); err != nil {
					return false, err
				}
				return false, nil
			}
		}

		var decodeErr error
		if sys.agentVersion.GTE(beszel.MinVersionCbor) {
			decodeErr = cbor.NewDecoder(stdout).Decode(sys.data)
		} else {
			decodeErr = json.NewDecoder(stdout).Decode(sys.data)
		}

		if decodeErr != nil {
			return true, decodeErr
		}

		if err := session.Wait(); err != nil {
			return false, err
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return sys.data, nil
}

// runSSHOperation establishes an SSH session and executes the provided operation.
// The operation can request a retry by returning true as the first return value.
func (sys *System) runSSHOperation(timeout time.Duration, retries int, operation func(*ssh.Session) (bool, error)) error {
	for attempt := 0; attempt <= retries; attempt++ {
		if sys.client == nil || sys.Status == down {
			if err := sys.createSSHClient(); err != nil {
				return err
			}
		}

		session, err := sys.createSessionWithTimeout(timeout)
		if err != nil {
			if attempt >= retries {
				return err
			}
			sys.manager.hub.Logger().Warn("Session closed. Retrying...", "host", sys.Host, "port", sys.Port, "err", err)
			sys.closeSSHConnection()
			continue
		}

		retry, opErr := func() (bool, error) {
			defer session.Close()
			return operation(session)
		}()

		if opErr == nil {
			return nil
		}

		if retry {
			sys.closeSSHConnection()
			if attempt < retries {
				continue
			}
		}

		return opErr
	}

	return fmt.Errorf("ssh operation failed")
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
// between 51% and 95% of the interval.
// This is used to stagger the initial WebSocket connections to prevent clustering.
func getJitter() <-chan time.Time {
	minPercent := 51
	maxPercent := 95
	jitterRange := maxPercent - minPercent
	msDelay := (interval * minPercent / 100) + rand.Intn(interval*jitterRange/100)
	return time.After(time.Duration(msDelay) * time.Millisecond)
}
