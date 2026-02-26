// Package ble provides a controller for managing Bluetooth Low Energy connections to BLEDOM devices.
package ble

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"bledom-controller/internal/core"

	"golang.org/x/time/rate"
	"tinygo.org/x/bluetooth"
)

var (
	adapter = bluetooth.DefaultAdapter

	defaultServiceUUIDStr        = "0000fff0-0000-1000-8000-00805f9b34fb"
	defaultCharacteristicUUIDStr = "0000fff3-0000-1000-8000-00805f9b34fb"
	scanStopGracePeriod          = 2 * time.Second

	errScanTimeout            = errors.New("ble scan timed out")
	errServiceNotFound        = errors.New("ble service not found")
	errCharacteristicNotFound = errors.New("ble characteristic not found")
)

// State represents the current logical state of the BLEDOM device.
type State struct {
	IsOn       bool
	R, G, B    int
	Brightness int
	Speed      int
}

// Controller manages the BLE connection, command queueing, and state tracking.
type Controller struct {
	characteristic bluetooth.DeviceCharacteristic
	heartbeatChar  bluetooth.DeviceCharacteristic

	disconnectChan chan struct{}
	commandChan    chan []byte

	deviceNames           []string
	bleServiceUUID        bluetooth.UUID
	bleCharacteristicUUID bluetooth.UUID
	bleScanTimeout        time.Duration
	bleConnectTimeout     time.Duration
	bleHeartbeatInterval  time.Duration
	bleRetryDelay         time.Duration
	bleCommandLimiter     *rate.Limiter

	eventBus *core.EventBus

	// --- State Management ---
	state   State
	stateMu sync.RWMutex

	unsupportedWriteOnce         sync.Once
	unsupportedHeartbeatReadOnce sync.Once
	unsupportedDisconnectOnce    sync.Once
}

// NewController creates and initializes a new BLE controller with the provided parameters.
func NewController(ctx context.Context, eb *core.EventBus, deviceNames []string, scanTimeout, connectTimeout, heartbeatInterval, retryDelay time.Duration, commandRateLimitRate float64, commandRateLimitBurst int) *Controller {
	serviceUUID, _ := bluetooth.ParseUUID(defaultServiceUUIDStr)
	characteristicUUID, _ := bluetooth.ParseUUID(defaultCharacteristicUUIDStr)

	c := &Controller{
		deviceNames:           deviceNames,
		bleServiceUUID:        serviceUUID,
		bleCharacteristicUUID: characteristicUUID,
		bleScanTimeout:        scanTimeout,
		bleConnectTimeout:     connectTimeout,
		bleHeartbeatInterval:  heartbeatInterval,
		bleRetryDelay:         retryDelay,
		commandChan:           make(chan []byte, commandRateLimitBurst*2),
		disconnectChan:        make(chan struct{}, 1),
		bleCommandLimiter:     rate.NewLimiter(rate.Limit(commandRateLimitRate), commandRateLimitBurst),
		eventBus:              eb,

		// Initial state
		state: State{
			IsOn:       false,
			R:          255,
			G:          255,
			B:          255,
			Brightness: 100,
			Speed:      50,
		},
	}

	go c.commandWriterLoop(ctx)
	return c
}

// GetState returns a thread-safe copy of the current device state.
func (c *Controller) GetState() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// Write enqueues a raw byte command to be sent to the device.
func (c *Controller) Write(payload []byte) {
	select {
	case c.commandChan <- payload:
	default:
		log.Printf("[BLE] Warning: Command queue full, dropping command: %x", payload)
	}
}

// commandWriterLoop is a background worker that processes and writes commands to the BLE characteristic.
func (c *Controller) commandWriterLoop(ctx context.Context) {
	log.Println("[BLE] Command writer loop started.")
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-c.commandChan:
			if err := c.bleCommandLimiter.Wait(ctx); err != nil {
				return
			}

			if c.characteristic.UUID() == (bluetooth.UUID{}) {
				continue
			}

			_, err := c.characteristic.WriteWithoutResponse(payload)
			if err != nil {
				if isUnsupportedWrite(err) {
					c.unsupportedWriteOnce.Do(func() {
						log.Printf("[BLE] Characteristic write is not supported by this device/backend. Temporarily disabling writes and forcing reconnect: %v", err)
					})
					c.characteristic = bluetooth.DeviceCharacteristic{}
					c.signalDisconnect()
					continue
				}
				log.Printf("[BLE] Failed to write to device (assuming disconnected): %v", err)
				c.signalDisconnect()
			}
		}
	}
}

// signalDisconnect safely triggers a reconnection attempt.
func (c *Controller) signalDisconnect() {
	select {
	case c.disconnectChan <- struct{}{}:
	default:
	}
}

// contains returns true if the specified string is present in the slice.
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func isMissingMethodError(err error, method string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, `Method "`+method+`"`) && strings.Contains(msg, "doesn't exist")
}

func isUnsupportedHeartbeatRead(err error) bool {
	return isMissingMethodError(err, "ReadValue")
}

func isUnsupportedWrite(err error) bool {
	return isMissingMethodError(err, "WriteValue")
}

func isUnsupportedDisconnect(err error) bool {
	return isMissingMethodError(err, "Disconnect")
}

func isLocalConnectionAbort(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "le-connection-abort-by-local") ||
		strings.Contains(msg, "connection-abort-by-local")
}

func (c *Controller) safeDisconnect(device bluetooth.Device) {
	if err := device.Disconnect(); err != nil {
		if isUnsupportedDisconnect(err) {
			c.unsupportedDisconnectOnce.Do(func() {
				log.Printf("[BLE] Disconnect method unsupported by backend. Ignoring: %v", err)
			})
			return
		}
		log.Printf("[BLE] Disconnect warning: %v", err)
	}
}

func (c *Controller) stopScanAndWait(scanDone <-chan struct{}) {
	_ = adapter.StopScan()

	select {
	case <-scanDone:
	case <-time.After(scanStopGracePeriod):
		log.Printf("[BLE] Warning: scan worker did not stop within %s", scanStopGracePeriod)
	}
}

func (c *Controller) scanForTargetDevice(ctx context.Context) (bluetooth.ScanResult, error) {
	scanResultChan := make(chan bluetooth.ScanResult, 1)
	scanErrChan := make(chan error, 1)
	scanDone := make(chan struct{})

	go func() {
		defer close(scanDone)
		err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			if !contains(c.deviceNames, result.LocalName()) {
				return
			}

			select {
			case scanResultChan <- result:
			default:
			}

			_ = adapter.StopScan()
		})
		if err != nil {
			select {
			case scanErrChan <- err:
			default:
			}
		}
	}()

	scanTimer := time.NewTimer(c.bleScanTimeout)
	defer scanTimer.Stop()

	select {
	case result := <-scanResultChan:
		c.stopScanAndWait(scanDone)
		return result, nil
	case err := <-scanErrChan:
		c.stopScanAndWait(scanDone)
		return bluetooth.ScanResult{}, err
	case <-scanTimer.C:
		c.stopScanAndWait(scanDone)
		return bluetooth.ScanResult{}, errScanTimeout
	case <-ctx.Done():
		c.stopScanAndWait(scanDone)
		return bluetooth.ScanResult{}, ctx.Err()
	}
}

func (c *Controller) discoverDeviceCharacteristics(device bluetooth.Device) error {
	services, err := device.DiscoverServices([]bluetooth.UUID{c.bleServiceUUID})
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return errServiceNotFound
	}

	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{c.bleCharacteristicUUID})
	if err != nil {
		return err
	}
	if len(chars) == 0 {
		return errCharacteristicNotFound
	}
	c.characteristic = chars[0]

	genericAccessUUID, _ := bluetooth.ParseUUID("00001800-0000-1000-8000-00805f9b34fb")
	deviceNameUUID, _ := bluetooth.ParseUUID("00002a00-0000-1000-8000-00805f9b34fb")
	gaServices, _ := device.DiscoverServices([]bluetooth.UUID{genericAccessUUID})
	if len(gaServices) > 0 {
		gaChars, _ := gaServices[0].DiscoverCharacteristics([]bluetooth.UUID{deviceNameUUID})
		if len(gaChars) > 0 {
			c.heartbeatChar = gaChars[0]
		}
	}

	return nil
}

// publishConnection publishes a connection event to the internal event bus.
func (c *Controller) publishConnection(connected bool, rssi int16) {
	if c.eventBus != nil {
		c.eventBus.Publish(core.Event{
			Type: core.DeviceConnectedEvent,
			Payload: map[string]interface{}{
				"connected": connected,
				"rssi":      rssi,
			},
		})
	}
}

// Run starts the main connection management loop, handling scanning, connecting, and discovery.
func (c *Controller) Run(ctx context.Context) {
	c.publishConnection(false, 0)

	for {
		select {
		case <-ctx.Done():
			log.Println("[BLE] Controller shutting down.")
			return
		default:
			// 1. Enable Adapter
			if err := adapter.Enable(); err != nil {
				log.Printf("[BLE] Failed to enable adapter: %v", err)
				time.Sleep(c.bleRetryDelay)
				continue
			}

			select {
			case <-c.disconnectChan:
			default:
			}

			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}

			log.Println("[BLE] Scanning for BLEDOM device...")
			_ = adapter.StopScan()

			var deviceScanResult bluetooth.ScanResult
			var scanErr error
			deviceScanResult, scanErr = c.scanForTargetDevice(ctx)
			if scanErr != nil {
				if errors.Is(scanErr, context.Canceled) {
					return
				}
				if errors.Is(scanErr, errScanTimeout) {
					log.Println("[BLE] Scan timed out. Retrying...")
				} else {
					log.Printf("[BLE] Scan error: %v", scanErr)
				}
				time.Sleep(c.bleRetryDelay)
				continue
			}
			log.Printf("[BLE] Found device: %s (RSSI: %d)", deviceScanResult.LocalName(), deviceScanResult.RSSI)

			log.Printf("[BLE] Connecting to %s...", deviceScanResult.Address.String())
			connectStartedAt := time.Now()
			device, err := adapter.Connect(deviceScanResult.Address, bluetooth.ConnectionParams{})
			if err != nil {
				if isLocalConnectionAbort(err) {
					log.Printf("[BLE] Connection aborted locally by adapter/backend. Retrying...")
					time.Sleep(c.bleRetryDelay)
					continue
				}
				log.Printf("[BLE] Failed to connect: %v", err)
				c.publishConnection(false, 0)
				time.Sleep(c.bleRetryDelay)
				continue
			}
			if connectElapsed := time.Since(connectStartedAt); connectElapsed > c.bleConnectTimeout {
				log.Printf("[BLE] Connect took %s (connect_timeout=%s)", connectElapsed.Round(time.Millisecond), c.bleConnectTimeout)
			}

			log.Printf("[BLE] Connected to %s", deviceScanResult.LocalName())
			c.publishConnection(true, deviceScanResult.RSSI)

			discoveryStartedAt := time.Now()
			if err := c.discoverDeviceCharacteristics(device); err != nil {
				log.Printf("[BLE] Service discovery failed: %v", err)
				c.publishConnection(false, 0)
				c.safeDisconnect(device)
				time.Sleep(c.bleRetryDelay)
				continue
			}
			if discoveryElapsed := time.Since(discoveryStartedAt); discoveryElapsed > c.bleConnectTimeout {
				log.Printf("[BLE] Discovery took %s (connect_timeout=%s)", discoveryElapsed.Round(time.Millisecond), c.bleConnectTimeout)
			}

			log.Println("[BLE] Device is ready.")

			heartbeatTicker := time.NewTicker(c.bleHeartbeatInterval)
			running := true
			heartbeatBuffer := make([]byte, 20)

			for running {
				select {
				case <-heartbeatTicker.C:
					if c.heartbeatChar.UUID() != (bluetooth.UUID{}) {
						_, err := c.heartbeatChar.Read(heartbeatBuffer)
						if err != nil {
							if isUnsupportedHeartbeatRead(err) {
								c.unsupportedHeartbeatReadOnce.Do(func() {
									log.Printf("[BLE] Heartbeat read is not supported by this device/backend. Disabling heartbeat read checks: %v", err)
								})
								c.heartbeatChar = bluetooth.DeviceCharacteristic{}
								continue
							}
							log.Printf("[BLE] Heartbeat failed: %v", err)
							c.signalDisconnect()
						}
					}
				case <-c.disconnectChan:
					log.Println("[BLE] Disconnection signal received. Resetting connection...")
					running = false

				case <-ctx.Done():
					log.Println("[BLE] Disconnecting due to shutdown...")
					c.safeDisconnect(device)
					return
				}
			}

			heartbeatTicker.Stop()
			c.publishConnection(false, 0)

			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}

			c.safeDisconnect(device)

			time.Sleep(c.bleRetryDelay)
		}
	}
}
