package systems

import (
	"beszel/internal/entities/system"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/store"
	"golang.org/x/crypto/ssh"
)

const (
	up      string = "up"
	down    string = "down"
	paused  string = "paused"
	pending string = "pending"

	// Update interval hub connect to agent in milliseconds to 30s
	interval int = 30_000

	sessionTimeout = 4 * time.Second
)

type SystemManager struct {
	hub       hubLike
	systems   *store.Store[string, *System]
	sshConfig *ssh.ClientConfig
}

type System struct {
	Id      string `db:"id"`
	Host    string `db:"host"`
	Port    string `db:"port"`
	Status  string `db:"status"`
	manager *SystemManager
	client  *ssh.Client
	data    *system.CombinedData
	ctx     context.Context
	cancel  context.CancelFunc
}

type hubLike interface {
	core.App
	GetSSHKey() ([]byte, error)
	HandleSystemAlerts(systemRecord *core.Record, data *system.CombinedData) error
	HandleStatusAlerts(status string, systemRecord *core.Record) error
}

func NewSystemManager(hub hubLike) *SystemManager {
	return &SystemManager{
		systems: store.New(map[string]*System{}),
		hub:     hub,
	}
}

// Initialize initializes the system manager.
// It binds the event hooks and starts updating existing systems.
func (sm *SystemManager) Initialize() error {
	sm.bindEventHooks()
	// ssh setup
	key, err := sm.hub.GetSSHKey()
	if err != nil {
		return err
	}
	if err := sm.createSSHClientConfig(key); err != nil {
		return err
	}
	// start updating existing systems
	var systems []*System
	err = sm.hub.DB().NewQuery("SELECT id, host, port, status FROM systems WHERE status != 'paused'").All(&systems)
	if err != nil || len(systems) == 0 {
		return err
	}
	go func() {
		// time between initial system updates
		delta := interval / max(1, len(systems))
		delta = min(delta, 2_000)
		sleepTime := time.Duration(delta) * time.Millisecond
		for _, system := range systems {
			time.Sleep(sleepTime)
			_ = sm.AddSystem(system)
		}
	}()
	return nil
}

func (sm *SystemManager) bindEventHooks() {
	sm.hub.OnRecordCreate("systems").BindFunc(sm.onRecordCreate)
	sm.hub.OnRecordAfterCreateSuccess("systems").BindFunc(sm.onRecordAfterCreateSuccess)
	sm.hub.OnRecordUpdate("systems").BindFunc(sm.onRecordUpdate)
	sm.hub.OnRecordAfterUpdateSuccess("systems").BindFunc(sm.onRecordAfterUpdateSuccess)
	sm.hub.OnRecordAfterDeleteSuccess("systems").BindFunc(sm.onRecordAfterDeleteSuccess)
}

// Runs before the record is committed to the database
func (sm *SystemManager) onRecordCreate(e *core.RecordEvent) error {
	e.Record.Set("info", system.Info{})
	e.Record.Set("status", pending)
	return e.Next()
}

// Runs after the record is committed to the database
func (sm *SystemManager) onRecordAfterCreateSuccess(e *core.RecordEvent) error {
	if err := sm.AddRecord(e.Record); err != nil {
		e.App.Logger().Error("Error adding record", "err", err)
	}
	return e.Next()
}

// Runs before the record is updated
func (sm *SystemManager) onRecordUpdate(e *core.RecordEvent) error {
	if e.Record.GetString("status") == paused {
		e.Record.Set("info", system.Info{})
	}
	return e.Next()
}

// Runs after the record is updated
func (sm *SystemManager) onRecordAfterUpdateSuccess(e *core.RecordEvent) error {
	newStatus := e.Record.GetString("status")
	switch newStatus {
	case paused:
		sm.RemoveSystem(e.Record.Id)
		return e.Next()
	case pending:
		if err := sm.AddRecord(e.Record); err != nil {
			e.App.Logger().Error("Error adding record", "err", err)
		}
		return e.Next()
	}
	system, ok := sm.systems.GetOk(e.Record.Id)
	if !ok {
		return sm.AddRecord(e.Record)
	}
	prevStatus := system.Status
	system.Status = newStatus
	// system alerts if system is up
	if system.Status == up {
		if err := sm.hub.HandleSystemAlerts(e.Record, system.data); err != nil {
			e.App.Logger().Error("Error handling system alerts", "err", err)
		}
	}
	if (system.Status == down && prevStatus == up) || (system.Status == up && prevStatus == down) {
		if err := sm.hub.HandleStatusAlerts(system.Status, e.Record); err != nil {
			e.App.Logger().Error("Error handling status alerts", "err", err)
		}
	}
	return e.Next()
}

// Runs after the record is deleted
func (sm *SystemManager) onRecordAfterDeleteSuccess(e *core.RecordEvent) error {
	sm.RemoveSystem(e.Record.Id)
	return e.Next()
}

// AddSystem adds a system to the manager
func (sm *SystemManager) AddSystem(sys *System) error {
	if sm.systems.Has(sys.Id) {
		return fmt.Errorf("system exists")
	}
	if sys.Id == "" || sys.Host == "" {
		return fmt.Errorf("system is missing required fields")
	}
	sys.manager = sm
	sys.ctx, sys.cancel = context.WithCancel(context.Background())
	sys.data = &system.CombinedData{}
	sm.systems.Set(sys.Id, sys)
	go sys.StartUpdater()
	return nil
}

// RemoveSystem removes a system from the manager
func (sm *SystemManager) RemoveSystem(systemID string) error {
	system, ok := sm.systems.GetOk(systemID)
	if !ok {
		return fmt.Errorf("system not found")
	}
	// cancel the context to signal stop
	if system.cancel != nil {
		system.cancel()
	}
	system.resetSSHClient()
	sm.systems.Remove(systemID)
	return nil
}

// AddRecord adds a record to the system manager.
// It first removes any existing system with the same ID, then creates a new System
// instance from the record data and adds it to the manager.
// This function is typically called when a new system is created or when an existing
// system's status changes to pending.
func (sm *SystemManager) AddRecord(record *core.Record) (err error) {
	_ = sm.RemoveSystem(record.Id)
	system := &System{
		Id:     record.Id,
		Status: record.GetString("status"),
		Host:   record.GetString("host"),
		Port:   record.GetString("port"),
	}
	return sm.AddSystem(system)
}

// StartUpdater starts the system updater.
// It first fetches the data from the agent then updates the records.
// If the data is not found or the system is down, it sets the system down.
func (sys *System) StartUpdater() {
	if sys.data == nil {
		sys.data = &system.CombinedData{}
	}
	if err := sys.update(); err != nil {
		_ = sys.setDown(err)
	}

	c := time.Tick(time.Duration(interval) * time.Millisecond)

	for {
		select {
		case <-sys.ctx.Done():
			return
		case <-c:
			err := sys.update()
			if err != nil {
				_ = sys.setDown(err)
			}
		}
	}
}

// update updates the system data and records.
// It first fetches the data from the agent then updates the records.
func (sys *System) update() error {
	_, err := sys.fetchDataFromAgent()
	if err == nil {
		_, err = sys.createRecords()
	}
	return err
}

// createRecords updates the system record and adds system_stats and container_stats records
func (sys *System) createRecords() (*core.Record, error) {
	systemRecord, err := sys.getRecord()
	if err != nil {
		return nil, err
	}
	hub := sys.manager.hub
	// add system_stats and container_stats records
	systemStats, err := hub.FindCachedCollectionByNameOrId("system_stats")
	if err != nil {
		return nil, err
	}
	systemStatsRecord := core.NewRecord(systemStats)
	systemStatsRecord.Set("system", systemRecord.Id)
	systemStatsRecord.Set("stats", sys.data.Stats)
	systemStatsRecord.Set("type", "1m")
	if err := hub.SaveNoValidate(systemStatsRecord); err != nil {
		return nil, err
	}
	// add new container_stats record
	if len(sys.data.Containers) > 0 {
		containerStats, err := hub.FindCachedCollectionByNameOrId("container_stats")
		if err != nil {
			return nil, err
		}
		containerStatsRecord := core.NewRecord(containerStats)
		containerStatsRecord.Set("system", systemRecord.Id)
		containerStatsRecord.Set("stats", sys.data.Containers)
		containerStatsRecord.Set("type", "1m")
		if err := hub.SaveNoValidate(containerStatsRecord); err != nil {
			return nil, err
		}
	}
	// update system record (do this last because it triggers alerts and we need above records to be inserted first)
	systemRecord.Set("status", up)
	systemRecord.Set("info", sys.data.Info)
	if err := hub.SaveNoValidate(systemRecord); err != nil {
		return nil, err
	}
	return systemRecord, nil
}

// getRecord retrieves the system record from the database.
// If the record is not found or the system is paused, it removes the system from the manager.
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
func (sys *System) setDown(OriginalError error) error {
	if sys.Status == down {
		return nil
	}
	record, err := sys.getRecord()
	if err != nil {
		return err
	}
	sys.manager.hub.Logger().Error("System down", "system", record.GetString("name"), "err", OriginalError)
	record.Set("status", down)
	err = sys.manager.hub.SaveNoValidate(record)
	if err != nil {
		return err
	}
	return nil
}

// fetchDataFromAgent fetches the data from the agent.
// It first creates a new SSH client if it doesn't exist or the system is down.
// Then it creates a new SSH session and fetches the data from the agent.
// If the data is not found or the system is down, it sets the system down.
func (sys *System) fetchDataFromAgent() (*system.CombinedData, error) {
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
			sys.resetSSHClient()
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

		// this is initialized in startUpdater, should never be nil
		*sys.data = system.CombinedData{}
		if err := json.NewDecoder(stdout).Decode(sys.data); err != nil {
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

func (sm *SystemManager) createSSHClientConfig(key []byte) error {
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}
	sm.sshConfig = &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sessionTimeout,
	}
	return nil
}

// createSSHClient creates a new SSH client for the system
func (s *System) createSSHClient() error {
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

// resetSSHClient closes the SSH connection and resets the client to nil
func (sys *System) resetSSHClient() {
	if sys.client != nil {
		sys.client.Close()
	}
	sys.client = nil
}
