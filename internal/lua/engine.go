// Package lua provides a Lua scripting engine for controlling BLE devices.
package lua

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/core"

	lua "github.com/yuin/gopher-lua"
)

// cmdType defines the type of engine command.
type cmdType int

const (
	cmdRunFile cmdType = iota
	cmdRunString
	cmdStop
)

// engineCmd represents a command sent to the Lua engine.
type engineCmd struct {
	kind cmdType
	name string
	code string
}

// Engine manages the Lua scripting environment using a single worker goroutine
// to ensure only one pattern runs at a time.
type Engine struct {
	bleController *ble.Controller
	patternsDir   string
	eventBus      *core.EventBus

	cmdChan chan engineCmd
	wg      sync.WaitGroup
}

// NewEngine creates a new Lua engine and starts its background worker.
func NewEngine(bleController *ble.Controller, patternsDir string, eb *core.EventBus) *Engine {
	e := &Engine{
		bleController: bleController,
		patternsDir:   patternsDir,
		eventBus:      eb,
		cmdChan:       make(chan engineCmd, 10),
	}

	go e.runLoop()

	return e
}

// runLoop is the main worker loop that processes engine commands sequentially.
func (e *Engine) runLoop() {
	var currentCancel context.CancelFunc
	var scriptDone chan struct{}

	for cmd := range e.cmdChan {
		if currentCancel != nil {
			currentCancel()
			select {
			case <-scriptDone:
			case <-time.After(2 * time.Second):
				log.Println("[Lua] Timeout waiting for script to stop")
			}
			currentCancel = nil
			scriptDone = nil
		}

		if cmd.kind == cmdStop {
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		currentCancel = cancel
		scriptDone = make(chan struct{})

		go func(cmd engineCmd, ctx context.Context, done chan struct{}) {
			switch cmd.kind {
			case cmdRunFile:
				e.executeFile(cmd.name, cmd.code, ctx, done)
			case cmdRunString:
				e.executeString(cmd.name, cmd.code, ctx, done)
			}
		}(cmd, ctx, scriptDone)
	}
}

// StopCurrentPattern stops the currently running script if any.
func (e *Engine) StopCurrentPattern() {
	select {
	case e.cmdChan <- engineCmd{kind: cmdStop}:
	default:
		log.Println("[Lua] Command channel full, could not send stop command")
	}
}

// RunPattern prepares and sends a command to execute a Lua script from a file.
func (e *Engine) RunPattern(name string) {
	scriptPath, err := e.GetPatternPath(name)
	if err != nil {
		log.Printf("[Lua] Could not get pattern path for '%s': %v", name, err)
		return
	}

	e.cmdChan <- engineCmd{
		kind: cmdRunFile,
		name: name,
		code: scriptPath,
	}
}

// ExecuteString prepares and sends a command to execute a one-off Lua command string.
func (e *Engine) ExecuteString(code string) {
	e.cmdChan <- engineCmd{
		kind: cmdRunString,
		name: "single line command",
		code: code,
	}
}

// sanitizeFilename checks for directory traversal and ensures a valid .lua extension.
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

// GetPatternPath returns the safe, absolute path to a pattern file within the engine's configured directory.
func (e *Engine) GetPatternPath(name string) (string, error) {
	cleanName, err := sanitizeFilename(name)
	if err != nil {
		return "", err
	}
	// Ensure the base directory exists
	if _, err := os.Stat(e.patternsDir); os.IsNotExist(err) {
		log.Printf("[Lua] Creating patterns directory: %s", e.patternsDir)
		if err := os.MkdirAll(e.patternsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create patterns directory: %w", err)
		}
	}
	return filepath.Join(e.patternsDir, cleanName), nil
}

// GetPatternCode reads and returns the source code of a pattern file.
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

// SavePatternCode writes the provided Lua source code to a pattern file.
func (e *Engine) SavePatternCode(name, code string) error {
	path, err := e.GetPatternPath(name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(code), 0644)
}

// DeletePattern removes a pattern file by name.
func (e *Engine) DeletePattern(name string) error {
	path, err := e.GetPatternPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// GetPatternList scans the patterns directory and returns a list of available .lua files.
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

// executeFile is an internal wrapper to run a Lua file within the worker's context.
func (e *Engine) executeFile(name, path string, ctx context.Context, done chan struct{}) {
	defer close(done)
	e.execute(name, func(L *lua.LState) error {
		return L.DoFile(path)
	}, ctx)
}

// executeString is an internal wrapper to run a Lua code string within the worker's context.
func (e *Engine) executeString(name, code string, ctx context.Context, done chan struct{}) {
	defer close(done)
	e.execute(name, func(L *lua.LState) error {
		return L.DoString(code)
	}, ctx)
}

// execute is a helper to run Lua code using a fresh state and provided executor function.
func (e *Engine) execute(name string, executor func(*lua.LState) error, ctx context.Context) {
	log.Printf("[Lua] Starting pattern '%s'...", name)
	if e.eventBus != nil {
		e.eventBus.Publish(core.Event{
			Type: core.PatternChangedEvent,
			Payload: map[string]interface{}{
				"running": name,
			},
		})
	}

	defer func() {
		log.Printf("[Lua] Pattern '%s' finished.", name)
		if e.eventBus != nil {
			e.eventBus.Publish(core.Event{
				Type: core.PatternChangedEvent,
				Payload: map[string]interface{}{
					"running": "",
				},
			})
		}
	}()

	L := lua.NewState()
	defer L.Close()
	L.SetContext(ctx)
	e.registerGoFunctions(L, ctx)

	if err := executor(L); err != nil {
		if ctx.Err() == context.Canceled {
			log.Printf("[Lua] Pattern '%s' execution was canceled.", name)
		} else {
			log.Printf("[Lua] Error executing pattern '%s': %v", name, err)
		}
	}
}
