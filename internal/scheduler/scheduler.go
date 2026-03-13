package scheduler

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"bledom-controller/internal/core"

	"github.com/robfig/cron/v3"
)

// ScheduleEntry defines the structure for a saved schedule.
type ScheduleEntry struct {
	Spec    string `json:"spec"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
	LastRun string `json:"last_run,omitempty"`
}

// ScheduleView defines the structure sent to the UI.
type ScheduleView struct {
	Spec    string `json:"spec"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
	LastRun string `json:"last_run,omitempty"`
	NextRun string `json:"next_run,omitempty"`
}

type scheduleFileEntry struct {
	Spec    string `json:"spec"`
	Command string `json:"command"`
	Enabled *bool  `json:"enabled,omitempty"`
	LastRun string `json:"last_run,omitempty"`
}

// Scheduler manages all cron-related tasks.
type Scheduler struct {
	cron           *cron.Cron
	store          map[cron.EntryID]ScheduleEntry
	commandChannel core.CommandChannel
	mu             sync.RWMutex
	schedulesFile  string
	onChange       func()
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

// SetOnChange registers a callback invoked when schedules change.
func (s *Scheduler) SetOnChange(fn func()) {
	s.onChange = fn
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
	entry := ScheduleEntry{Spec: spec, Command: command, Enabled: true}
	id, err := s.addJobLocked(entry)
	if err != nil {
		s.mu.Unlock()
		log.Printf("Error adding schedule '%s' '%s': %v", spec, command, err)
		return
	}
	s.save()
	s.mu.Unlock()
	log.Printf("Added schedule (ID %d): %s -> %s", id, spec, command)
	s.notifyChange()
}

// Remove deletes a cron job.
func (s *Scheduler) Remove(id int) {
	s.mu.Lock()

	entryID := cron.EntryID(id)
	s.cron.Remove(entryID)
	delete(s.store, entryID)
	s.save()
	s.mu.Unlock()
	log.Printf("Removed schedule (ID %d)", id)
	s.notifyChange()
}

// Update modifies an existing schedule by removing and re-adding it.
func (s *Scheduler) Update(id int, spec, command string) {
	entryID := cron.EntryID(id)

	s.mu.Lock()
	entry, ok := s.store[entryID]
	if !ok {
		s.mu.Unlock()
		log.Printf("Schedule (ID %d) not found for update.", id)
		return
	}

	newEntry := ScheduleEntry{
		Spec:    spec,
		Command: command,
		Enabled: entry.Enabled,
		LastRun: "",
	}

	newID, err := s.addJobLocked(newEntry)
	if err != nil {
		s.mu.Unlock()
		log.Printf("Error updating schedule '%s' '%s': %v", spec, command, err)
		return
	}

	s.cron.Remove(entryID)
	delete(s.store, entryID)
	s.save()
	s.mu.Unlock()

	log.Printf("Updated schedule (ID %d -> %d): %s -> %s", id, newID, spec, command)
	s.notifyChange()
}

// SetEnabled toggles a schedule on or off without removing it.
func (s *Scheduler) SetEnabled(id int, enabled bool) {
	entryID := cron.EntryID(id)
	s.mu.Lock()
	entry, ok := s.store[entryID]
	if !ok {
		s.mu.Unlock()
		log.Printf("Schedule (ID %d) not found for enable toggle.", id)
		return
	}
	entry.Enabled = enabled
	s.store[entryID] = entry
	s.save()
	s.mu.Unlock()
	log.Printf("Set schedule (ID %d) enabled=%v", id, enabled)
	s.notifyChange()
}

// SetAllEnabled toggles all schedules.
func (s *Scheduler) SetAllEnabled(enabled bool) {
	s.mu.Lock()
	changed := false
	for id, entry := range s.store {
		if entry.Enabled != enabled {
			entry.Enabled = enabled
			s.store[id] = entry
			changed = true
		}
	}
	if changed {
		s.save()
	}
	s.mu.Unlock()
	if changed {
		log.Printf("Set all schedules enabled=%v", enabled)
		s.notifyChange()
	}
}

// RunNow executes a schedule immediately.
func (s *Scheduler) RunNow(id int) {
	entryID := cron.EntryID(id)
	s.mu.RLock()
	entry, ok := s.store[entryID]
	s.mu.RUnlock()
	if !ok {
		log.Printf("Schedule (ID %d) not found for run now.", id)
		return
	}
	s.execute(entryID, entry.Command, true)
}

// GetAll returns a copy of the current schedules in a thread-safe way.
func (s *Scheduler) GetAll() map[cron.EntryID]ScheduleView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	newMap := make(map[cron.EntryID]ScheduleView)
	for id, entry := range s.store {
		view := ScheduleView{
			Spec:    entry.Spec,
			Command: entry.Command,
			Enabled: entry.Enabled,
			LastRun: entry.LastRun,
		}
		if entry.Enabled {
			cronEntry := s.cron.Entry(id)
			if !cronEntry.Next.IsZero() {
				view.NextRun = cronEntry.Next.Local().Format(time.RFC3339)
			}
		}
		newMap[id] = view
	}
	return newMap
}

func (s *Scheduler) execute(id cron.EntryID, command string, force bool) {
	if !force {
		s.mu.RLock()
		entry, ok := s.store[id]
		s.mu.RUnlock()
		if !ok || !entry.Enabled {
			return
		}
	}

	now := time.Now().Local().Format(time.RFC3339)
	s.mu.Lock()
	if entry, ok := s.store[id]; ok {
		entry.LastRun = now
		s.store[id] = entry
		s.save()
	}
	s.mu.Unlock()
	s.notifyChange()

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
	fileStore := make(map[cron.EntryID]scheduleFileEntry)
	for id, entry := range s.store {
		enabled := entry.Enabled
		fileStore[id] = scheduleFileEntry{
			Spec:    entry.Spec,
			Command: entry.Command,
			Enabled: &enabled,
			LastRun: entry.LastRun,
		}
	}
	data, err := json.MarshalIndent(fileStore, "", "  ")
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

	tempStore := make(map[cron.EntryID]scheduleFileEntry)
	if err := json.Unmarshal(data, &tempStore); err != nil {
		log.Printf("Error unmarshalling schedule file: %v", err)
		return
	}

	log.Printf("Loading %d schedules from file '%s'...", len(tempStore), s.schedulesFile)
	for _, entry := range tempStore {
		jobEntry := entry
		enabled := true
		if jobEntry.Enabled != nil {
			enabled = *jobEntry.Enabled
		}
		storeEntry := ScheduleEntry{
			Spec:    jobEntry.Spec,
			Command: jobEntry.Command,
			Enabled: enabled,
			LastRun: jobEntry.LastRun,
		}
		if _, err := s.addJobLocked(storeEntry); err != nil {
			log.Printf("Error re-adding schedule from file: %v", err)
			continue
		}
	}
}

func (s *Scheduler) addJobLocked(entry ScheduleEntry) (cron.EntryID, error) {
	var entryID cron.EntryID
	entryID, err := s.cron.AddFunc(entry.Spec, func() { s.execute(entryID, entry.Command, false) })
	if err != nil {
		return 0, err
	}
	s.store[entryID] = entry
	return entryID, nil
}

func (s *Scheduler) notifyChange() {
	if s.onChange != nil {
		s.onChange()
	}
}
