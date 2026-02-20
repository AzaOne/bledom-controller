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

func (e *Engine) runLoop() {
	var currentCancel context.CancelFunc
	var scriptDone chan struct{}

	for cmd := range e.cmdChan {
		if currentCancel != nil {
			currentCancel()
			select {
			case <-scriptDone:
			case <-time.After(2 * time.Second):
				log.Println("Lua engine: timeout waiting for script to stop")
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

// StopCurrentPattern stops the currently running script.
func (e *Engine) StopCurrentPattern() {
	select {
	case e.cmdChan <- engineCmd{kind: cmdStop}:
	default:
		log.Println("Lua engine: command channel full, could not send stop")
	}
}

// RunPattern prepares and sends a command to execute a Lua script from a file.
func (e *Engine) RunPattern(name string) {
	scriptPath, err := e.GetPatternPath(name)
	if err != nil {
		log.Printf("Could not get pattern path: %v", err)
		return
	}

	e.cmdChan <- engineCmd{
		kind: cmdRunFile,
		name: name,
		code: scriptPath,
	}
}

// ExecuteString prepares and sends a command to execute a one-off Lua command.
func (e *Engine) ExecuteString(code string) {
	e.cmdChan <- engineCmd{
		kind: cmdRunString,
		name: "single line command",
		code: code,
	}
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

// executeFile is the internal method called within a goroutine.
func (e *Engine) executeFile(name, path string, ctx context.Context, done chan struct{}) {
	defer close(done)
	e.execute(name, func(L *lua.LState) error {
		return L.DoFile(path)
	}, ctx)
}

// executeString is the internal method called within a goroutine.
func (e *Engine) executeString(name, code string, ctx context.Context, done chan struct{}) {
	defer close(done)
	e.execute(name, func(L *lua.LState) error {
		return L.DoString(code)
	}, ctx)
}

// execute is a helper to run Lua code with a context.
func (e *Engine) execute(name string, executor func(*lua.LState) error, ctx context.Context) {
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
	}()

	L := lua.NewState()
	defer L.Close()
	L.SetContext(ctx)
	e.registerGoFunctions(L, ctx)

	if err := executor(L); err != nil {
		if ctx.Err() == context.Canceled {
			log.Printf("Lua pattern '%s' execution was canceled.", name)
		} else {
			log.Printf("Error executing Lua pattern '%s': %v", name, err)
		}
	}
}
