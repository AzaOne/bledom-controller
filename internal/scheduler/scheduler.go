package scheduler

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"

	"bledom-controller/internal/core"

	"github.com/robfig/cron/v3"
)

// ScheduleEntry defines the structure for a saved schedule.
type ScheduleEntry struct {
	Spec    string `json:"spec"`
	Command string `json:"command"`
}

// Scheduler manages all cron-related tasks.
type Scheduler struct {
	cron           *cron.Cron
	store          map[cron.EntryID]ScheduleEntry
	commandChannel core.CommandChannel
	mu             sync.RWMutex
	schedulesFile  string
}

// NewScheduler creates and loads a scheduler.
func NewScheduler(cmdChan core.CommandChannel, schedulesFile string) *Scheduler {
	s := &Scheduler{
		cron:           cron.New(),
		store:          make(map[cron.EntryID]ScheduleEntry),
		commandChannel: cmdChan,
		schedulesFile:  schedulesFile,
	}
	s.load()
	return s
}

// Start begins the cron job ticker.
func (s *Scheduler) Start() {
	s.cron.Start()
	log.Println("Cron scheduler started.")
}

// Stop halts the cron job ticker.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Cron scheduler stopped.")
}

// Add creates a new cron job.
func (s *Scheduler) Add(spec, command string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.cron.AddFunc(spec, func() { s.execute(command) })
	if err != nil {
		log.Printf("Error adding schedule '%s' '%s': %v", spec, command, err)
		return
	}
	s.store[id] = ScheduleEntry{Spec: spec, Command: command}
	s.save()
	log.Printf("Added schedule (ID %d): %s -> %s", id, spec, command)
}

// Remove deletes a cron job.
func (s *Scheduler) Remove(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID := cron.EntryID(id)
	s.cron.Remove(entryID)
	delete(s.store, entryID)
	s.save()
	log.Printf("Removed schedule (ID %d)", id)
}

// GetAll returns a copy of the current schedules in a thread-safe way.
func (s *Scheduler) GetAll() map[cron.EntryID]ScheduleEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	newMap := make(map[cron.EntryID]ScheduleEntry)
	for k, v := range s.store {
		newMap[k] = v
	}
	return newMap
}

func (s *Scheduler) execute(command string) {
	log.Printf("Executing scheduled command: %s", command)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "power":
		if len(parts) > 1 && parts[1] == "on" {
			s.commandChannel <- core.Command{Type: core.CmdSetPower, Payload: map[string]interface{}{"isOn": true}}
		} else {
			s.commandChannel <- core.Command{Type: core.CmdSetPower, Payload: map[string]interface{}{"isOn": false}}
		}
	case "pattern":
		if len(parts) > 1 {
			s.commandChannel <- core.Command{Type: core.CmdRunPattern, Payload: map[string]interface{}{"name": parts[1]}}
		}
	case "lua":
		// LUA dynamic execute disabled in schedule to pure command struct mappings.
		// It could be re-implemented similarly if there's a CmdExecuteLua type.
		log.Printf("Lua dynamic code from schedule is deprecated via pure commands: %s", command)
	}
}

func (s *Scheduler) save() {
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		log.Printf("Error marshalling schedules: %v", err)
		return
	}
	os.WriteFile(s.schedulesFile, data, 0644)
}

func (s *Scheduler) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.schedulesFile); os.IsNotExist(err) {
		return
	}
	data, err := os.ReadFile(s.schedulesFile)
	if err != nil {
		log.Printf("Error reading schedule file: %v", err)
		return
	}

	tempStore := make(map[cron.EntryID]ScheduleEntry)
	if err := json.Unmarshal(data, &tempStore); err != nil {
		log.Printf("Error unmarshalling schedule file: %v", err)
		return
	}

	log.Printf("Loading %d schedules from file '%s'...", len(tempStore), s.schedulesFile)
	for _, entry := range tempStore {
		jobEntry := entry
		newID, err := s.cron.AddFunc(jobEntry.Spec, func() { s.execute(jobEntry.Command) })
		if err != nil {
			log.Printf("Error re-adding schedule from file: %v", err)
			continue
		}
		s.store[newID] = jobEntry
	}
}
