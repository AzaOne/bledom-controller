package lua

import (
	"context"
	"log"
	"math"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// registerGoFunctions exposes Go functions to the given Lua state.
func (e *Engine) registerGoFunctions(L *lua.LState, cancellable bool) {
	L.SetGlobal("set_color", L.NewFunction(e.luaSetColor))
	L.SetGlobal("set_brightness", L.NewFunction(e.luaSetBrightness))
	L.SetGlobal("set_power", L.NewFunction(e.luaSetPower))
	L.SetGlobal("print", L.NewFunction(luaPrint))

	if cancellable {
		L.SetGlobal("sleep", L.NewFunction(e.luaSleepCancellable))
		L.SetGlobal("should_stop", L.NewFunction(e.luaShouldStop))

		// NEW: Register high-level effect functions
		L.SetGlobal("breathe", L.NewFunction(e.luaBreathe))
		L.SetGlobal("strobe", L.NewFunction(e.luaStrobe))
		L.SetGlobal("fade", L.NewFunction(e.luaFade))
	} else {
		L.SetGlobal("sleep", L.NewFunction(luaSleepNoCancel))
	}
}

func luaPrint(L *lua.LState) int {
	log.Printf("[LUA] %s", L.ToString(1))
	return 0
}

func (e *Engine) luaSetColor(L *lua.LState) int {
	r, g, b := L.ToInt(1), L.ToInt(2), L.ToInt(3)
	e.bleController.SetColor(r, g, b)
	return 0
}

func (e *Engine) luaSetBrightness(L *lua.LState) int {
	e.bleController.SetBrightness(L.ToInt(1))
	return 0
}

func (e *Engine) luaSetPower(L *lua.LState) int {
	e.bleController.SetPower(L.ToBool(1))
	return 0
}

// cancellableSleep is a helper to sleep for a duration, but wake up immediately if the context is cancelled.
// It returns true if the context was cancelled during sleep.
func cancellableSleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return false
	case <-ctx.Done():
		return true
	}
}

func (e *Engine) luaSleepCancellable(L *lua.LState) int {
	ms := L.ToInt(1)
	if cancellableSleep(e.currentPatternCtx, time.Duration(ms)*time.Millisecond) {
		return 0
	}
	return 0
}

func luaSleepNoCancel(L *lua.LState) int {
	ms := L.ToInt(1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return 0
}

func (e *Engine) luaShouldStop(L *lua.LState) int {
	select {
	case <-e.currentPatternCtx.Done():
		L.Push(lua.LBool(true))
	default:
		L.Push(lua.LBool(false))
	}
	return 1
}

// luaBreathe performs a smooth pulse animation from 1% to 100% and back to 1% brightness
// over the specified duration. The color should be set before calling this function.
func (e *Engine) luaBreathe(L *lua.LState) int {
	durationMs := L.ToInt(1)
	duration := time.Duration(durationMs) * time.Millisecond

	steps := 100
	stepDuration := duration / time.Duration(2*steps)

	// Fade in
	for i := 1; i <= steps; i++ {
		e.bleController.SetBrightness(i)
		if cancellableSleep(e.currentPatternCtx, stepDuration) {
			return 0
		}
	}

	// Fade out
	for i := steps; i >= 1; i-- {
		e.bleController.SetBrightness(i)
		if cancellableSleep(e.currentPatternCtx, stepDuration) {
			return 0
		}
	}
	return 0
}

// luaStrobe flashes a specific color for a total duration at a given frequency (in Hz).
func (e *Engine) luaStrobe(L *lua.LState) int {
	r := L.ToInt(1)
	g := L.ToInt(2)
	b := L.ToInt(3)
	durationMs := L.ToInt(4)
	hz := L.ToNumber(5)

	duration := time.Duration(durationMs) * time.Millisecond

	e.bleController.SetPower(true)
	e.bleController.SetBrightness(100)

	// Calculate the on/off time for each flash to match the frequency
	if hz <= 0 {
		return 0
	} // Avoid division by zero
	halfPeriod := time.Duration(1000/hz/2) * time.Millisecond
	startTime := time.Now()

	for time.Since(startTime) < duration {
		e.bleController.SetColor(r, g, b)
		if cancellableSleep(e.currentPatternCtx, halfPeriod) {
			return 0
		}

		// Turn off for a better strobe effect
		e.bleController.SetColor(0, 0, 0)
		if cancellableSleep(e.currentPatternCtx, halfPeriod) {
			return 0
		}
	}
	return 0
}

// luaFade smoothly transitions from a starting color to an ending color over a duration.
func (e *Engine) luaFade(L *lua.LState) int {
	r1 := L.ToInt(1)
	g1 := L.ToInt(2)
	b1 := L.ToInt(3)
	r2 := L.ToInt(4)
	g2 := L.ToInt(5)
	b2 := L.ToInt(6)
	durationMs := L.ToInt(7)

	duration := time.Duration(durationMs) * time.Millisecond

	e.bleController.SetPower(true)

	steps := 100
	stepDuration := duration / time.Duration(steps)

	for i := 0; i <= steps; i++ {
		progress := float64(i) / float64(steps)

		// Linear interpolation for each color channel
		r := int(math.Round(float64(r1) + progress*(float64(r2-r1))))
		g := int(math.Round(float64(g1) + progress*(float64(g2-g1))))
		b := int(math.Round(float64(b1) + progress*(float64(b2-b1))))

		e.bleController.SetColor(r, g, b)

		if cancellableSleep(e.currentPatternCtx, stepDuration) {
			return 0
		}
	}
	// Ensure the final color is set exactly
	e.bleController.SetColor(r2, g2, b2)
	return 0
}
