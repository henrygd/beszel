package hub

import (
	"beszel/internal/entities/system"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

type SystemManager struct {
	sync.Mutex                    // used to lock updateMap
	updateMap  map[string]*System // map keyed by system ID
	hub        *Hub
	sshConfig  *ssh.ClientConfig
}

type System struct {
	Id           string `db:"id"`
	Host         string `db:"host"`
	Port         string `db:"port"`
	client       *ssh.Client
	data         *system.CombinedData
	stopChan     chan struct{}
	manager      *SystemManager
	paused       bool
	retries      uint8
	needsRefresh bool
}

func NewSystemManager(hub *Hub) *SystemManager {
	return &SystemManager{
		hub:       hub,
		updateMap: make(map[string]*System),
	}
}

func (sm *SystemManager) createSSHClientConfig() error {
	key, err := sm.hub.GetSSHKey()
	if err != nil {
		return err
	}
	// Create the Signer for this private key.
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
		Timeout:         4 * time.Second,
	}
	return nil
}

func (sm *SystemManager) Initialize() {
	err := sm.createSSHClientConfig()
	if err != nil {
		return
	}
	// systemCount, err := sm.hub.CountRecords("systems")
	var systems []*System
	err = sm.hub.DB().NewQuery("SELECT id, host, port FROM systems").All(&systems)
	if err != nil || len(systems) == 0 {
		return
	}
	// time between initial system updates
	delta := max(2_000, 60_000/len(systems))
	sleepTime := time.Duration(delta) * time.Millisecond
	log.Println("delta", delta)
	// TODO: race condition - user starts hub and deletes system within one minute. system is still here and will still be added
	for _, system := range systems {
		time.Sleep(sleepTime)
		_ = sm.AddSystem(system)
	}

}

func (sm *SystemManager) AddSystem(system *System) error {
	sm.Lock()
	defer sm.Unlock()
	_, ok := sm.updateMap[system.Id]
	if ok { // overwrite - we should only call add system if  this should be if pending - delete system from map and start over - or should we just manually delete system from pending check before after update... yes probably
		return fmt.Errorf("system exists")
	}
	if system.Id == "" || system.Host == "" {
		return fmt.Errorf("system is missing required fields")
	}
	system.manager = sm
	system.stopChan = make(chan struct{})
	sm.updateMap[system.Id] = system
	go system.StartUpdater()
	return nil
}

// when system is updated;
// 1. system manager delete (stop goroutine, close connection, remove from map)

func (sm *SystemManager) DeleteSystem(systemID string) error {
	sm.Lock()
	defer sm.Unlock()
	system, exists := sm.updateMap[systemID]
	if !exists {
		return fmt.Errorf("system not found")
	}
	system.stopChan <- struct{}{}
	if system.client != nil {
		system.client.Close()
	}
	delete(sm.updateMap, system.Id)
	return nil
}

func (s *System) CreateSSHClient() error {
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
	return nil
}

func (sys *System) StartUpdater() {
	if err := sys.CreateSSHClient(); err != nil {
		sys.setDown("Failed to connect:", "system", sys.Id, "err", err)
	}
	c := time.Tick(60 * time.Second)

	for {
		select {
		case <-sys.stopChan:
			return
		case <-c:
			if sys.paused {
				continue
			}
			err := sys.update()
			if err != nil {
				sys.setDown("Failed to update", "system", sys.Id, "err", err)
				sys.needsRefresh = true
			}
		}
	}
}

func (sm *SystemManager) Upsert(record *core.Record) error {
	sm.Lock()
	defer sm.Unlock()
	system, exists := sm.updateMap[record.Id]
	if !exists {
		return sm.AddSystem(&System{Id: record.Id, Host: record.GetString("host"), Port: record.GetString("port")})
	}
	system.Host = record.GetString("host")
	system.Port = record.GetString("port")
	system.paused = false
	system.needsRefresh = true
	return nil
}

func (sm *SystemManager) PauseSystem(systemID string) error {
	sm.Lock()
	defer sm.Unlock()
	system, exists := sm.updateMap[systemID]
	if !exists {
		// TODO: handle - maybe make system
		return fmt.Errorf("system not found")
	}
	system.paused = true
	return nil
}

// NOTE: 20 second - should we store last update time and only update if more than 50 seconds have passed? and change the updater to run every 20 seconds
// this way down systems update sooner - down status updates will have time to rerun a few times

func (sys *System) update() error {
	if sys.client == nil || sys.needsRefresh {
		if err := sys.CreateSSHClient(); err != nil {
			return err
		}
	}
	session, err := newSessionWithTimeout(sys.client, 4*time.Second)
	if err != nil {
		if sys.retries > 0 {
			sys.retries = 0
			return err
		}
		sys.retries++
		sys.manager.hub.Logger().Warn("Existing SSH connection closed. Retrying...", "host", sys.Host, "port", sys.Port)
		sys.client = nil
		// TODO: better pattern to get rid of recursion
		return sys.update()
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := session.Shell(); err != nil {
		return err
	}
	*sys.data = system.CombinedData{}
	if err := json.NewDecoder(stdout).Decode(sys.data); err != nil {
		return err
	}
	// wait for the session to complete
	if err := session.Wait(); err != nil {
		return err
	}

	// update system record
	hub := sys.manager.hub
	record, err := hub.FindRecordById("systems", sys.Id)
	if err != nil {
		sys.manager.DeleteSystem(sys.Id)
		return err
	}
	record.Set("status", "up")
	record.Set("info", sys.data.Info)
	if err := hub.SaveNoValidate(record); err != nil {
		return err
	}
	// add system_stats and container_stats records
	systemStats, err := hub.FindCachedCollectionByNameOrId("system_stats")
	if err != nil {
		return err
	}
	systemStatsRecord := core.NewRecord(systemStats)
	systemStatsRecord.Set("system", record.Id)
	systemStatsRecord.Set("stats", sys.data.Stats)
	systemStatsRecord.Set("type", "1m")
	if err := hub.SaveNoValidate(systemStatsRecord); err != nil {
		return err
	}
	// add new container_stats record
	if len(sys.data.Containers) > 0 {
		containerStats, err := hub.FindCachedCollectionByNameOrId("container_stats")
		if err != nil {
			return err
		}
		containerStatsRecord := core.NewRecord(containerStats)
		containerStatsRecord.Set("system", record.Id)
		containerStatsRecord.Set("stats", sys.data.Containers)
		containerStatsRecord.Set("type", "1m")
		if err := hub.SaveNoValidate(containerStatsRecord); err != nil {
			return err
		}
	}
	// system info alerts
	if err := hub.am.HandleSystemAlerts(record, sys.data.Info, sys.data.Stats.Temperatures, sys.data.Stats.ExtraFs); err != nil {
		hub.Logger().Error("System alerts error", "err", err.Error())
	}
	return nil
}

func (sys *System) setDown(msg string, args ...any) error {
	record, err := sys.manager.hub.FindRecordById("systems", sys.Id)
	if err != nil {
		return err
	}
	if record == nil || record.GetString("status") == "down" {
		return nil
	}
	record.Set("status", "down")
	sys.manager.hub.Logger().Error(msg, args...)
	return sys.manager.hub.SaveNoValidate(record)
}
