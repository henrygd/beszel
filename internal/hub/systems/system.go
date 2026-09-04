package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/transport"
	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/henrygd/beszel/internal/entities/zfs"

	"github.com/henrygd/beszel"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

type System struct {
	Id             string                     `db:"id"`
	Host           string                     `db:"host"`
	Port           string                     `db:"port"`
	Status         string                     `db:"status"`
	manager        *SystemManager             // Manager that this system belongs to
	client         atomic.Pointer[ssh.Client] // SSH client for fetching data
	sshTransport   *transport.SSHTransport    // SSH transport for requests
	data           *system.CombinedData       // system data from agent
	ctx            context.Context            // Context for stopping the updater
	cancel         context.CancelFunc         // Stops and removes system from updater
	WsConn         *ws.WsConn                 // Handler for agent WebSocket connection
	agentVersion   semver.Version             // Agent version
	updateTicker   *time.Ticker               // Ticker for updating the system
	detailsFetched atomic.Bool                // True if static system details have been fetched and saved
	smartFetching  atomic.Bool                // True if SMART devices are currently being fetched
	smartInterval  time.Duration              // Interval for periodic SMART data updates
	zfsFetching    atomic.Bool                // True if ZFS pools are currently being fetched
	zfsInterval    time.Duration              // Interval for periodic ZFS detail data updates
}

func (sm *SystemManager) NewSystem(systemId string) *System {
	system := &System{
		Id:   systemId,
		data: &system.CombinedData{},
	}
	system.ctx, system.cancel = system.getContext(sm.ctx)
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
		if !waitForContext(sys.ctx, 11*time.Second) {
			return
		}

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
	// fetch system details if not already fetched
	if !sys.detailsFetched.Load() {
		options.IncludeDetails = true
	}

	data, err := sys.fetchDataFromAgent(options)
	if err != nil {
		return err
	}

	// ensure deprecated fields from older agents are migrated to current fields
	migrateDeprecatedFields(data, !sys.detailsFetched.Load())

	// create system records
	_, err = sys.createRecords(data)

	// if details were included and fetched successfully, mark details as fetched and update smart interval if set by agent
	if err == nil && data.Details != nil {
		sys.detailsFetched.Store(true)
		// update smart interval if it's set on the agent side
		if data.Details.SmartInterval > 0 {
			sys.smartInterval = data.Details.SmartInterval
			sys.manager.hub.Logger().Info("SMART interval updated from agent details", "system", sys.Id, "interval", sys.smartInterval.String())
			// make sure we reset expiration of lastFetch to remain as long as the new smart interval
			// to prevent premature expiration leading to new fetch if interval is different.
			sys.manager.smartFetchMap.UpdateExpiration(sys.Id, sys.smartInterval+time.Minute)
		}
		// update zfs interval if it's set on the agent side
		if data.Details.ZfsInterval > 0 {
			sys.zfsInterval = data.Details.ZfsInterval
			sys.manager.hub.Logger().Info("ZFS interval updated from agent details", "system", sys.Id, "interval", sys.zfsInterval.String())
			sys.manager.zfsFetchMap.UpdateExpiration(sys.Id, sys.zfsInterval+time.Minute)
		}
	}

	// Fetch and save SMART devices when system first comes online or at intervals
	if backgroundSmartFetchEnabled() && sys.detailsFetched.Load() {
		if sys.smartInterval <= 0 {
			sys.smartInterval = time.Hour
		}
		if sys.shouldFetchSmart() && sys.smartFetching.CompareAndSwap(false, true) {
			sys.manager.hub.Logger().Info("SMART fetch", "system", sys.Id, "interval", sys.smartInterval.String())
			go func() {
				defer sys.smartFetching.Store(false)
				_ = sys.FetchAndSaveSmartDevices()
			}()
		}
	}

	// Fetch and save ZFS pool details when system first comes online or at intervals
	if backgroundZfsFetchEnabled() && sys.detailsFetched.Load() && sys.supportsZfsData() {
		if sys.zfsInterval <= 0 {
			sys.zfsInterval = time.Hour
		}
		if sys.shouldFetchZfs() && sys.zfsFetching.CompareAndSwap(false, true) {
			sys.manager.hub.Logger().Info("ZFS fetch", "system", sys.Id, "interval", sys.zfsInterval.String())
			go func() {
				defer sys.zfsFetching.Store(false)
				_ = sys.FetchAndSaveZfsPools(false)
			}()
		}
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
	systemRecord, err := sys.getRecord(sys.manager.hub)
	if err != nil {
		return nil, err
	}
	hub := sys.manager.hub
	err = hub.RunInTransaction(func(txApp core.App) error {
		// add system_stats record
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

		// add containers and container_stats records
		if len(data.Containers) > 0 {
			if data.Containers[0].Id != "" {
				if err := createContainerRecords(txApp, data.Containers, sys.Id); err != nil {
					return err
				}
			}
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

		// Update systemd service records when the agent reports a fresh snapshot.
		// The length check keeps snapshots from older agents working, while the
		// explicit marker lets newer agents report that a fresh snapshot is empty.
		if data.SystemdServicesUpdated || len(data.SystemdServices) > 0 {
			if err := createSystemdStatsRecords(txApp, data.SystemdServices, sys.Id); err != nil {
				return err
			}
		}

		// add system details record
		if data.Details != nil {
			if err := createSystemDetailsRecord(txApp, data.Details, sys.Id); err != nil {
				return err
			}
		}

		if err := sys.syncZfsPoolHealth(txApp, data.Stats.ZfsPools); err != nil {
			return err
		}

		// update system record (do this last because it triggers alerts and we need above records to be inserted first)
		systemRecord.Set("status", up)
		systemRecord.Set("info", data.Info)
		if err := txApp.SaveNoValidate(systemRecord); err != nil {
			return err
		}
		return nil
	})

	return systemRecord, err
}

func createSystemDetailsRecord(app core.App, data *system.Details, systemId string) error {
	collectionName := "system_details"
	params := dbx.Params{
		"id":       systemId,
		"system":   systemId,
		"hostname": data.Hostname,
		"kernel":   data.Kernel,
		"cores":    data.Cores,
		"threads":  data.Threads,
		"cpu":      data.CpuModel,
		"os":       data.Os,
		"os_name":  data.OsName,
		"arch":     data.Arch,
		"memory":   data.MemoryTotal,
		"podman":   data.Podman,
		"updated":  time.Now().UTC(),
	}
	result, err := app.DB().Update(collectionName, params, dbx.HashExp{"id": systemId}).Execute()
	rowsAffected, _ := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		_, err = app.DB().Insert(collectionName, params).Execute()
	}
	return err
}

func createSystemdStatsRecords(app core.App, data []*systemd.Service, systemId string) error {
	if len(data) == 0 {
		_, err := app.DB().NewQuery(
			"DELETE FROM systemd_services WHERE system = {:system}",
		).Bind(dbx.Params{"system": systemId}).Execute()
		return err
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
	if _, err := app.DB().NewQuery(queryString).Bind(params).Execute(); err != nil {
		return err
	}
	// Remove services the agent no longer reports. Every row in this batch shares the
	// same updated timestamp, so anything older no longer exists on the host. Left in
	// place these rows survive until the retention sweep and surface inconsistently
	// across the dashboard, the services table, and alerts.
	_, err := app.DB().NewQuery(
		"DELETE FROM systemd_services WHERE system = {:system} AND updated < {:updated}",
	).Bind(dbx.Params{"system": systemId, "updated": params["updated"]}).Execute()
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
		valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:image%[1]s}, {:ports%[1]s}, {:status%[1]s}, {:health%[1]s}, {:cpu%[1]s}, {:memory%[1]s}, {:net%[1]s}, {:updated})", suffix))
		params["id"+suffix] = container.Id
		params["name"+suffix] = container.Name
		params["image"+suffix] = container.Image
		params["ports"+suffix] = container.Ports
		params["status"+suffix] = container.Status
		params["health"+suffix] = container.Health
		params["cpu"+suffix] = container.Cpu
		params["memory"+suffix] = container.Mem
		netBytes := container.Bandwidth[0] + container.Bandwidth[1]
		if netBytes == 0 {
			netBytes = uint64((container.NetworkSent + container.NetworkRecv) * 1024 * 1024)
		}
		params["net"+suffix] = netBytes
	}
	queryString := fmt.Sprintf(
		"INSERT INTO containers (id, system, name, image, ports, status, health, cpu, memory, net, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system = excluded.system, name = excluded.name, image = excluded.image, ports = excluded.ports, status = excluded.status, health = excluded.health, cpu = excluded.cpu, memory = excluded.memory, net = excluded.net, updated = excluded.updated",
		strings.Join(valueStrings, ","),
	)
	_, err := app.DB().NewQuery(queryString).Bind(params).Execute()
	return err
}

// getRecord retrieves the system record from the database.
// If the record is not found, it removes the system from the manager.
func (sys *System) getRecord(app core.App) (*core.Record, error) {
	record, err := app.FindRecordById("systems", sys.Id)
	if err != nil || record == nil {
		_ = sys.manager.RemoveSystem(sys.Id)
		if err == nil {
			err = fmt.Errorf("system record %s not found", sys.Id)
		}
		return nil, err
	}
	return record, nil
}

// HasUser checks if the given user is in the system's users list.
// Returns true if SHARE_ALL_SYSTEMS is enabled (any authenticated user can access any system).
func (sys *System) HasUser(app core.App, user *core.Record) bool {
	if user == nil {
		return false
	}
	if v, _ := utils.GetEnv("SHARE_ALL_SYSTEMS"); v == "true" {
		return true
	}
	var recordData = struct {
		Users string
	}{}
	err := app.DB().NewQuery("SELECT users FROM systems WHERE id={:id}").
		Bind(dbx.Params{"id": sys.Id}).
		One(&recordData)
	if err != nil || recordData.Users == "" {
		return false
	}
	return strings.Contains(recordData.Users, user.Id)
}

// setDown marks a system as down in the database.
// It takes the original error that caused the system to go down and returns any error
// encountered during the process of updating the system status.
// It is a no-op if the system's context has been cancelled.
func (sys *System) setDown(originalError error) error {
	if sys.Status == down || sys.Status == paused {
		return nil
	}
	// the updater can race shutdown, and the app may already be disposed by the
	// time we get here, so don't touch the database once the context is cancelled
	if sys.ctx != nil && sys.ctx.Err() != nil {
		return sys.ctx.Err()
	}
	record, err := sys.getRecord(sys.manager.hub)
	if err != nil {
		return err
	}
	if originalError != nil {
		sys.manager.hub.Logger().Error("System down", "system", record.GetString("name"), "err", originalError)
	}
	sys.detailsFetched.Store(false)
	record.Set("status", down)
	return sys.manager.hub.SaveNoValidate(record)
}

func (sys *System) getContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if sys.ctx == nil {
		sys.ctx, sys.cancel = context.WithCancel(ctx)
	}
	return sys.ctx, sys.cancel
}

// request sends a request to the agent, trying WebSocket first, then SSH.
// This is the unified request method that uses the transport abstraction.
func (sys *System) request(ctx context.Context, action common.WebSocketAction, req any, dest any) error {
	// Try WebSocket first
	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		wsTransport := transport.NewWebSocketTransport(sys.WsConn)
		if err := wsTransport.Request(ctx, action, req, dest); err == nil {
			return nil
		} else if !shouldFallbackToSSH(err) {
			return err
		} else if shouldCloseWebSocket(err) {
			sys.closeWebSocketConnection()
		}
	}

	// Fall back to SSH if WebSocket fails
	if err := sys.ensureSSHTransport(); err != nil {
		return err
	}
	err := sys.sshTransport.RequestWithRetry(ctx, action, req, dest, 1)
	// Keep legacy SSH client/version fields in sync for other code paths.
	if sys.sshTransport != nil {
		sys.client.Store(sys.sshTransport.GetClient())
		sys.agentVersion = sys.sshTransport.GetAgentVersion()
	}
	return err
}

func shouldFallbackToSSH(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, gws.ErrConnClosed) {
		return true
	}
	return errors.Is(err, transport.ErrWebSocketNotConnected)
}

func shouldCloseWebSocket(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gws.ErrConnClosed) || errors.Is(err, transport.ErrWebSocketNotConnected)
}

// ensureSSHTransport ensures the SSH transport is initialized and connected.
func (sys *System) ensureSSHTransport() error {
	if sys.sshTransport == nil {
		if sys.manager.sshConfig == nil {
			if err := sys.manager.createSSHClientConfig(); err != nil {
				return err
			}
		}
		sys.sshTransport = transport.NewSSHTransport(transport.SSHTransportConfig{
			Host:    sys.Host,
			Port:    sys.Port,
			Config:  sys.manager.sshConfig,
			Timeout: 4 * time.Second,
		})
	}
	// Sync client state with transport
	if client := sys.client.Load(); client != nil {
		sys.sshTransport.SetClient(client)
		sys.sshTransport.SetAgentVersion(sys.agentVersion)
	}
	return nil
}

// fetchDataFromAgent attempts to fetch data from the agent, prioritizing WebSocket if available.
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
	wsTransport := transport.NewWebSocketTransport(sys.WsConn)
	err := wsTransport.Request(context.Background(), common.GetData, options, sys.data)
	if err != nil {
		return nil, err
	}
	return sys.data, nil
}

// FetchContainerInfoFromAgent fetches container info from the agent
func (sys *System) FetchContainerInfoFromAgent(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	err := sys.request(ctx, common.GetContainerInfo, common.ContainerInfoRequest{ContainerID: containerID}, &result)
	return result, err
}

// FetchContainerLogsFromAgent fetches container logs from the agent
func (sys *System) FetchContainerLogsFromAgent(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	err := sys.request(ctx, common.GetContainerLogs, common.ContainerLogsRequest{ContainerID: containerID}, &result)
	return result, err
}

// FetchSystemdInfoFromAgent fetches detailed systemd service information from the agent
func (sys *System) FetchSystemdInfoFromAgent(serviceName string) (systemd.ServiceDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result systemd.ServiceDetails
	err := sys.request(ctx, common.GetSystemdInfo, common.SystemdInfoRequest{ServiceName: serviceName}, &result)
	return result, err
}

// FetchSmartDataFromAgent fetches SMART data from the agent.
func (sys *System) FetchSmartDataFromAgent() (smart.SmartDataResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if sys.agentVersion.LT(beszel.MinVersionAgentResponse) {
		var data map[string]smart.SmartData
		err := sys.request(ctx, common.GetSmartData, nil, &data)
		return smart.SmartDataResponse{Data: data}, err
	}
	var result smart.SmartDataResponse
	err := sys.request(ctx, common.GetSmartData, nil, &result)
	return result, err
}

// FetchZfsDataFromAgent fetches ZFS detail data from the agent.
func (sys *System) FetchZfsDataFromAgent(force bool) (*zfs.ZfsData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var result zfs.ZfsData
	err := sys.request(ctx, common.GetZfsData, common.ZfsDataRequest{Force: force}, &result)
	return &result, err
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
			req := common.HubRequest[any]{Action: common.GetData, Data: options}
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
		if sys.client.Load() == nil || sys.Status == down {
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

		// Bound the whole operation. A half-open TCP connection (a dead peer that
		// never sends RST/FIN) or a wedged agent that accepts the session but
		// never writes a response would otherwise block the read forever. Because
		// StartUpdater runs update() synchronously on its ticker, that stalls the
		// per-system updater indefinitely with no error and no re-dial until the
		// hub is restarted (issue #2041). On timeout we tear down the connection
		// so the blocked read unwinds and the system is re-dialed on the next tick.
		retry, opErr := runWithTimeout(sshOperationTimeout, func() (bool, error) {
			defer session.Close()
			return operation(session)
		}, sys.closeSSHConnection)

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

// sshOperationTimeout bounds a single SSH data exchange (send request, read
// response, wait for the remote command to exit). It is more generous than the
// session-creation timeout to tolerate briefly slow agents, but is kept well
// under the collection interval so a stalled connection is detected and
// re-dialed within one cycle (see issue #2041).
const sshOperationTimeout = 20 * time.Second

// runWithTimeout runs op in a goroutine and returns its result, or, if op does
// not finish within timeout, calls onTimeout (used to tear down the connection
// so a blocked op can unwind) and returns a retryable timeout error. This
// guarantees the caller can never block indefinitely on a dead SSH connection.
func runWithTimeout(timeout time.Duration, op func() (bool, error), onTimeout func()) (retry bool, err error) {
	type opResult struct {
		retry bool
		err   error
	}
	// Buffered so the op goroutine never leaks even when we return on timeout.
	done := make(chan opResult, 1)
	go func() {
		r, e := op()
		done <- opResult{retry: r, err: e}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-done:
		return res.retry, res.err
	case <-timer.C:
		if onTimeout != nil {
			onTimeout()
		}
		return true, fmt.Errorf("ssh operation timed out after %s", timeout)
	}
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
	client, err := dialSSHWithKeepAlive(network, host, s.manager.sshConfig)
	s.client.Store(client)
	if err != nil {
		return err
	}
	s.agentVersion, _ = extractAgentVersion(string(client.Conn.ServerVersion()))
	s.manager.resetFailedSmartFetchState(s.Id)
	s.manager.resetFailedZfsFetchState(s.Id)
	return nil
}

// sshKeepAliveInterval is the TCP keep-alive idle interval for SSH connections
// to agents. Enabling OS-level keep-alives lets the hub eventually detect a
// dead peer on an otherwise idle connection instead of trusting it forever.
// This is a backstop for genuine network death; an application-level wedge
// (agent process hung while its kernel keeps ACKing) is caught by the
// per-operation timeout in runSSHOperation instead (see issue #2041).
const sshKeepAliveInterval = 30 * time.Second

// dialSSHWithKeepAlive dials an SSH connection like ssh.Dial, but enables TCP
// keep-alive on the underlying connection so half-open connections are
// eventually detected by the operating system.
func dialSSHWithKeepAlive(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	dialer := net.Dialer{
		Timeout:   config.Timeout,
		KeepAlive: sshKeepAliveInterval,
	}
	conn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

// createSessionWithTimeout creates a new SSH session with a timeout to avoid hanging
// in case of network issues
func (sys *System) createSessionWithTimeout(timeout time.Duration) (*ssh.Session, error) {
	client := sys.client.Load()
	if client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	ctx, cancel := context.WithTimeout(sys.ctx, timeout)
	defer cancel()

	sessionChan := make(chan *ssh.Session, 1)
	errChan := make(chan error, 1)

	go func() {
		if session, err := client.NewSession(); err != nil {
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
	if sys.sshTransport != nil {
		sys.sshTransport.Close()
	}
	if client := sys.client.Swap(nil); client != nil {
		client.Close()
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

// migrateDeprecatedFields moves values from deprecated fields to their new locations if the new
// fields are not already populated. Deprecated fields and refs may be removed at least 30 days
// and one minor version release after the release that includes the migration.
//
// This is run when processing incoming system data from agents, which may be on older versions.
func migrateDeprecatedFields(cd *system.CombinedData, createDetails bool) {
	// migration added 0.19.0
	if cd.Stats.Bandwidth[0] == 0 && cd.Stats.Bandwidth[1] == 0 {
		cd.Stats.Bandwidth[0] = uint64(cd.Stats.NetworkSent * 1024 * 1024)
		cd.Stats.Bandwidth[1] = uint64(cd.Stats.NetworkRecv * 1024 * 1024)
		cd.Stats.NetworkSent, cd.Stats.NetworkRecv = 0, 0
	}
	// migration added 0.19.0
	if cd.Info.BandwidthBytes == 0 {
		cd.Info.BandwidthBytes = uint64(cd.Info.Bandwidth * 1024 * 1024)
		cd.Info.Bandwidth = 0
	}
	// migration added 0.19.0
	if cd.Stats.DiskIO[0] == 0 && cd.Stats.DiskIO[1] == 0 {
		cd.Stats.DiskIO[0] = uint64(cd.Stats.DiskReadPs * 1024 * 1024)
		cd.Stats.DiskIO[1] = uint64(cd.Stats.DiskWritePs * 1024 * 1024)
		cd.Stats.DiskReadPs, cd.Stats.DiskWritePs = 0, 0
	}
	// migration added 0.19.0 - Move deprecated Info fields to Details struct
	if cd.Details == nil && cd.Info.Hostname != "" {
		if createDetails {
			cd.Details = &system.Details{
				Hostname:    cd.Info.Hostname,
				Kernel:      cd.Info.KernelVersion,
				Cores:       cd.Info.Cores,
				Threads:     cd.Info.Threads,
				CpuModel:    cd.Info.CpuModel,
				Podman:      cd.Info.Podman,
				Os:          cd.Info.Os,
				MemoryTotal: uint64(cd.Stats.Mem * 1024 * 1024 * 1024),
			}
		}
		// zero the deprecated fields to prevent saving them in systems.info DB json payload
		cd.Info.Hostname = ""
		cd.Info.KernelVersion = ""
		cd.Info.Cores = 0
		cd.Info.CpuModel = ""
		cd.Info.Podman = false
		cd.Info.Os = 0
	}
}
