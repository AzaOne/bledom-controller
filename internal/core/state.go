package core

import "sync"

// State holds the single source of truth for the device.
type State struct {
	mu             sync.RWMutex
	IsConnected    bool
	RSSI           int16
	Power          bool
	ColorR         int
	ColorG         int
	ColorB         int
	Brightness     int
	Speed          int
	RunningPattern string
}

// NewState creates a new State instance.
func NewState() *State {
	return &State{}
}

// Clone returns a snapshot of the current state for safe reading.
func (s *State) Clone() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return State{
		IsConnected:    s.IsConnected,
		RSSI:           s.RSSI,
		Power:          s.Power,
		ColorR:         s.ColorR,
		ColorG:         s.ColorG,
		ColorB:         s.ColorB,
		Brightness:     s.Brightness,
		Speed:          s.Speed,
		RunningPattern: s.RunningPattern,
	}
}

// SetConnection updates connection state.
func (s *State) SetConnection(connected bool, rssi int16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsConnected = connected
	s.RSSI = rssi
}

// SetPower updates the power state.
func (s *State) SetPower(power bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Power = power
}

// SetColor updates the RGB color state.
func (s *State) SetColor(r, g, b int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ColorR = r
	s.ColorG = g
	s.ColorB = b
}

// SetBrightness updates the brightness state.
func (s *State) SetBrightness(brightness int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Brightness = brightness
}

// SetSpeed updates the speed state.
func (s *State) SetSpeed(speed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Speed = speed
}

// SetRunningPattern updates the running pattern state.
func (s *State) SetRunningPattern(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RunningPattern = pattern
}
