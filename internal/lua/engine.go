package lua

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/core"

	lua "github.com/yuin/gopher-lua"
)

// scriptJob represents a request to execute a Lua script.
type scriptJob struct {
	name     string
	executor func(*lua.LState) error
}

// Engine manages the Lua scripting environment using a single worker goroutine
// to ensure only one pattern runs at a time.
type Engine struct {
	bleController        *ble.Controller
	jobQueue             chan scriptJob
	currentPatternCtx    context.Context
	currentPatternCancel context.CancelFunc
	patternMutex         sync.Mutex

	patternsDir string
	eventBus    *core.EventBus
}

// NewEngine creates a new Lua engine and starts its background worker.
func NewEngine(bleController *ble.Controller, patternsDir string, eb *core.EventBus) *Engine {
	e := &Engine{
		bleController: bleController,
		jobQueue:      make(chan scriptJob, 1),
		patternsDir:   patternsDir, // Store patterns directory
		eventBus:      eb,
	}
	go e.worker()
	return e
}

// worker is the heart of the engine. It runs in a single goroutine,
// processing jobs from the jobQueue one by one. This serial execution
// prevents all race conditions.
func (e *Engine) worker() {
	for job := range e.jobQueue {
		e.execute(job.name, job.executor)
	}
}

// StopCurrentPattern stops the currently running script and clears any pending script from the queue.
func (e *Engine) StopCurrentPattern() {
	log.Println("Received stop command.")
	select {
	case <-e.jobQueue:
		log.Println("Cleared a pending job from the queue.")
	default:
	}

	e.patternMutex.Lock()
	defer e.patternMutex.Unlock()
	if e.currentPatternCancel != nil {
		log.Println("Stopping currently running pattern via context cancellation.")
		e.currentPatternCancel()
		e.currentPatternCancel = nil
	}
}

// sendJob sends a job to the worker, replacing any job that's already waiting.
func (e *Engine) sendJob(job scriptJob) {
	select {
	case <-e.jobQueue:
		log.Println("Replaced a pending job in the queue.")
	default:
	}
	e.jobQueue <- job
}

// sanitizeFilename checks for directory traversal and valid extension.
func sanitizeFilename(name string) (string, error) {
	if !strings.HasSuffix(name, ".lua") {
		return "", fmt.Errorf("filename must end with .lua")
	}
	cleanName := filepath.Base(name)
	if cleanName == "" || cleanName == ".lua" || strings.Contains(cleanName, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	return cleanName, nil
}

// GetPatternPath returns the safe, absolute path to a pattern file using the engine's configured directory.
func (e *Engine) GetPatternPath(name string) (string, error) {
	cleanName, err := sanitizeFilename(name)
	if err != nil {
		return "", err
	}
	// Ensure the base directory exists
	if _, err := os.Stat(e.patternsDir); os.IsNotExist(err) {
		log.Printf("Creating patterns directory: %s", e.patternsDir)
		os.Mkdir(e.patternsDir, 0755)
	}
	return filepath.Join(e.patternsDir, cleanName), nil
}

// GetPatternCode reads the content of a pattern file.
func (e *Engine) GetPatternCode(name string) (string, error) {
	path, err := e.GetPatternPath(name)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SavePatternCode writes content to a pattern file.
func (e *Engine) SavePatternCode(name, code string) error {
	path, err := e.GetPatternPath(name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(code), 0644)
}

// DeletePattern removes a pattern file.
func (e *Engine) DeletePattern(name string) error {
	path, err := e.GetPatternPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// GetPatternList returns a slice of all available pattern filenames.
func (e *Engine) GetPatternList() ([]string, error) {
	var patterns []string
	files, err := os.ReadDir(e.patternsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return patterns, nil
		}
		return nil, err
	}
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".lua" {
			patterns = append(patterns, file.Name())
		}
	}
	return patterns, nil
}

// RunPattern prepares and sends a job to execute a Lua script from a file.
func (e *Engine) RunPattern(name string) {
	e.StopCurrentPattern()

	scriptPath, err := e.GetPatternPath(name)
	if err != nil {
		log.Printf("Could not get pattern path: %v", err)
		return
	}

	executor := func(L *lua.LState) error {
		return L.DoFile(scriptPath)
	}

	e.sendJob(scriptJob{
		name:     name,
		executor: executor,
	})
}

// ExecuteString prepares and sends a job to execute a one-off Lua command.
func (e *Engine) ExecuteString(code string) {
	e.StopCurrentPattern()

	executor := func(L *lua.LState) error {
		return L.DoString(code)
	}

	e.sendJob(scriptJob{
		name:     "single line command",
		executor: executor,
	})
}

// execute is the internal method called ONLY by the worker goroutine.
func (e *Engine) execute(name string, executor func(*lua.LState) error) {
	e.patternMutex.Lock()
	if e.currentPatternCancel != nil {
		log.Printf("Execute: Stopping previous pattern before starting '%s'", name)
		e.currentPatternCancel()
	}
	e.patternMutex.Unlock()

	e.patternMutex.Lock()
	e.currentPatternCtx, e.currentPatternCancel = context.WithCancel(context.Background())
	e.patternMutex.Unlock()

	log.Printf("Starting pattern '%s'...", name)
	if e.eventBus != nil {
		e.eventBus.Publish(core.Event{
			Type: core.PatternChangedEvent,
			Payload: map[string]interface{}{
				"pattern": name,
			},
		})
	}

	defer func() {
		log.Printf("Pattern '%s' finished.", name)
		if e.eventBus != nil {
			e.eventBus.Publish(core.Event{
				Type: core.PatternChangedEvent,
				Payload: map[string]interface{}{
					"pattern": "",
				},
			})
		}

		e.patternMutex.Lock()
		e.currentPatternCancel = nil
		e.patternMutex.Unlock()
	}()

	L := lua.NewState()
	defer L.Close()
	e.registerGoFunctions(L, true)

	if err := executor(L); err != nil {
		if e.currentPatternCtx.Err() == context.Canceled {
			log.Printf("Lua pattern '%s' execution was canceled.", name)
		} else {
			log.Printf("Error executing Lua pattern '%s': %v", name, err)
		}
	}
}
