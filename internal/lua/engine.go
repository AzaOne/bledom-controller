package lua

import (
	"context"
	"log"
	"sync"

	"bledom-controller/internal/ble"
	lua "github.com/yuin/gopher-lua"
)

// scriptJob represents a request to execute a Lua script.
type scriptJob struct {
	name           string
	onStatusChange func(string)
	executor       func(*lua.LState) error
}

// Engine manages the Lua scripting environment using a single worker goroutine
// to ensure only one pattern runs at a time.
type Engine struct {
	bleController *ble.Controller
	
	// jobQueue is a buffered channel of size 1 used to send requests to the worker.
	// The buffer allows us to replace a pending job with a new one.
	jobQueue chan scriptJob

	// These fields are managed exclusively by the worker goroutine.
	currentPatternCtx    context.Context
	currentPatternCancel context.CancelFunc
	
	// Mutex is still needed for StopCurrentPattern, which can be called from other goroutines.
	patternMutex sync.Mutex
}

// NewEngine creates a new Lua engine and starts its background worker.
func NewEngine(bleController *ble.Controller) *Engine {
	e := &Engine{
		bleController: bleController,
		// Buffer size of 1 is crucial for the "replace" logic.
		jobQueue:      make(chan scriptJob, 1),
	}
	// Start the single worker goroutine that will process all jobs sequentially.
	go e.worker()
	return e
}

// worker is the heart of the engine. It runs in a single goroutine,
// processing jobs from the jobQueue one by one. This serial execution
// prevents all race conditions.
func (e *Engine) worker() {
	for job := range e.jobQueue {
		// The actual execution logic is now encapsulated here.
		e.execute(job.name, job.onStatusChange, job.executor)
	}
}

// StopCurrentPattern stops the currently running script and clears any pending script from the queue.
func (e *Engine) StopCurrentPattern() {
	log.Println("Received stop command.")
	// Step 1: Clear any job that is waiting in the queue.
	// This is a non-blocking read. If the channel is empty, it does nothing.
	select {
	case <-e.jobQueue:
		log.Println("Cleared a pending job from the queue.")
	default:
		// Queue was empty, nothing to clear.
	}

	// Step 2: Cancel the currently executing script, if any.
	e.patternMutex.Lock()
	defer e.patternMutex.Unlock()
	if e.currentPatternCancel != nil {
		e.currentPatternCancel()
		e.currentPatternCancel = nil // Prevent multiple cancellations
		log.Println("Canceled the context of the running pattern.")
	}
}

// sendJob sends a job to the worker, replacing any job that's already waiting.
func (e *Engine) sendJob(job scriptJob) {
	// Non-blocking read to clear any pending job. This ensures that if the user
	// clicks buttons rapidly, only the very last action is executed.
	select {
	case <-e.jobQueue:
		log.Println("Replaced a pending job in the queue.")
	default:
		// Queue was empty.
	}
	// Send the new job to the worker.
	e.jobQueue <- job
}

// RunPattern prepares and sends a job to execute a Lua script from a file.
func (e *Engine) RunPattern(name string, onStatusChange func(runningPattern string)) {
	// Immediately stop any currently running or pending pattern.
	e.StopCurrentPattern()

	scriptPath, err := GetPatternPath(name)
	if err != nil {
		log.Printf("Could not get pattern path: %v", err)
		return
	}

	executor := func(L *lua.LState) error {
		return L.DoFile(scriptPath)
	}

	e.sendJob(scriptJob{
		name:           name,
		onStatusChange: onStatusChange,
		executor:       executor,
	})
}

// ExecuteString prepares and sends a job to execute a one-off Lua command.
func (e *Engine) ExecuteString(code string, onStatusChange func(runningPattern string)) {
	// Also stop any running pattern when a scheduled command comes in.
	e.StopCurrentPattern()

	executor := func(L *lua.LState) error {
		return L.DoString(code)
	}

	e.sendJob(scriptJob{
		name:           "single line command",
		onStatusChange: onStatusChange, // Pass the callback to the job
		executor:       executor,
	})
}

// execute is the internal method called ONLY by the worker goroutine.
// It contains the logic for running a single script.
func (e *Engine) execute(name string, onStatusChange func(string), executor func(*lua.LState) error) {
	// First, ensure any previous pattern is fully stopped.
	// This is now slightly redundant because StopCurrentPattern is called earlier,
	// but it's a harmless and safe check.
	e.patternMutex.Lock()
	if e.currentPatternCancel != nil {
		e.currentPatternCancel()
	}
	e.patternMutex.Unlock()

	// Set up the context for the new pattern.
	e.patternMutex.Lock()
	e.currentPatternCtx, e.currentPatternCancel = context.WithCancel(context.Background())
	e.patternMutex.Unlock()

	log.Printf("Starting pattern '%s'...", name)
	if onStatusChange != nil {
		onStatusChange(name)
	}

	defer func() {
		log.Printf("Pattern '%s' finished.", name)
		if onStatusChange != nil {
			onStatusChange("")
		}
		
		e.patternMutex.Lock()
		e.currentPatternCancel = nil // Clean up the cancel function
		e.patternMutex.Unlock()
	}()
	
	L := lua.NewState()
	defer L.Close()
	e.registerGoFunctions(L, true)

	if err := executor(L); err != nil {
		// Avoid logging an error if the script was intentionally canceled.
		if e.currentPatternCtx.Err() != context.Canceled {
			log.Printf("Error executing Lua pattern '%s': %v", name, err)
		}
	}
}
