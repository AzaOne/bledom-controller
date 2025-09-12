package scheduler

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/lua"
	"github.com/robfig/cron/v3"
)

const scheduleFile = "schedules.json"

// ScheduleEntry defines the structure for a saved schedule.
type ScheduleEntry struct {
	Spec    string `json:"spec"`
	Command string `json:"command"`
}

// Scheduler manages all cron-related tasks.
type Scheduler struct {
	cron          *cron.Cron
	store         map[cron.EntryID]ScheduleEntry
	luaEngine     *lua.Engine
	bleController *ble.Controller
	mu            sync.RWMutex
	onPatternStatusChange func(string)

}

// NewScheduler creates and loads a scheduler.
func NewScheduler(le *lua.Engine, bc *ble.Controller, onStatusChange func(string)) *Scheduler {
	s := &Scheduler{
		cron:                  cron.New(),
		store:                 make(map[cron.EntryID]ScheduleEntry),
		luaEngine:             le,
		bleController:         bc,
		onPatternStatusChange: onStatusChange,
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
	// Return a copy to prevent race conditions on the returned map
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
		s.luaEngine.StopCurrentPattern()
		if len(parts) > 1 && parts[1] == "on" {
			s.bleController.SetPower(true)
		} else {
			s.bleController.SetPower(false)
		}
	case "pattern":
		if len(parts) > 1 {
			// Pass the scheduler's status callback to the luaEngine.
			go s.luaEngine.RunPattern(parts[1], s.onPatternStatusChange)
		}
	case "lua":
		luaCode := strings.Join(parts[1:], " ")
		if luaCode != "" {
			// Pass the scheduler's status callback to the luaEngine.
			go s.luaEngine.ExecuteString(luaCode, s.onPatternStatusChange)
		}
	}
}

func (s *Scheduler) save() {
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		log.Printf("Error marshalling schedules: %v", err)
		return
	}
	ioutil.WriteFile(scheduleFile, data, 0644)
}

func (s *Scheduler) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(scheduleFile); os.IsNotExist(err) {
		return
	}
	data, err := ioutil.ReadFile(scheduleFile)
	if err != nil {
		log.Printf("Error reading schedule file: %v", err)
		return
	}
	
	// Create a temporary map to unmarshal into
	tempStore := make(map[cron.EntryID]ScheduleEntry)
	if err := json.Unmarshal(data, &tempStore); err != nil {
		log.Printf("Error unmarshalling schedule file: %v", err)
		return
	}

	log.Printf("Loading %d schedules from file...", len(tempStore))
	// Replaced 'id' with the blank identifier '_'
	for _, entry := range tempStore {
		// Use a temporary variable in the loop to avoid capturing the wrong 'entry' in the closure.
		jobEntry := entry
		newID, err := s.cron.AddFunc(jobEntry.Spec, func() { s.execute(jobEntry.Command) })
		if err != nil {
			log.Printf("Error re-adding schedule from file: %v", err)
			continue
		}
		// Store with the new ID, as IDs can change between runs.
		s.store[newID] = jobEntry
	}
}
