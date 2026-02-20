package ble

import (
	"context"
	"log"
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
)

// State зберігає поточний стан пристрою
type State struct {
	IsOn       bool
	R, G, B    int
	Brightness int
	Speed      int
}

// Controller manages the BLE connection and commands.
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

	// --- Збереження стану ---
	state   State
	stateMu sync.RWMutex
}

// NewController creates a new BLE controller.
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

		// Початковий стан
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

// GetState повертає копію поточного стану (thread-safe)
func (c *Controller) GetState() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// Write sends a byte command.
func (c *Controller) Write(payload []byte) {
	select {
	case c.commandChan <- payload:
	default:
		log.Printf("Warning: BLE command queue full, dropping command: %x", payload)
	}
}

// commandWriterLoop processes commands and writes to BLE.
func (c *Controller) commandWriterLoop(ctx context.Context) {
	log.Println("BLE command writer loop started.")
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
				log.Printf("Failed to write to BLEDOM (assuming disconnected): %v", err)
				c.signalDisconnect()
			}
		}
	}
}

// signalDisconnect safely sends a disconnect signal.
func (c *Controller) signalDisconnect() {
	select {
	case c.disconnectChan <- struct{}{}:
	default:
	}
}

// contains checks if a string is in a slice.
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// publishConnection is a helper to publish connection events.
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

// Run starts the BLE connection management loop.
func (c *Controller) Run(ctx context.Context) {
	c.publishConnection(false, 0)

	for {
		select {
		case <-ctx.Done():
			log.Println("BLE controller shutting down.")
			return
		default:
			// 1. Enable Adapter
			if err := adapter.Enable(); err != nil {
				log.Printf("Failed to enable adapter: %v", err)
				time.Sleep(c.bleRetryDelay)
				continue
			}

			select {
			case <-c.disconnectChan:
			default:
			}

			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}

			log.Println("Scanning for BLEDOM device...")
			adapter.StopScan()

			ch := make(chan bluetooth.ScanResult, 1)

			go func() {
				err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
					if contains(c.deviceNames, result.LocalName()) {
						adapter.StopScan()
						select {
						case ch <- result:
						default:
						}
					}
				})
				if err != nil {
					log.Printf("Scan error: %v", err)
				}
			}()

			var deviceScanResult bluetooth.ScanResult
			scanCtx, cancelScan := context.WithTimeout(ctx, c.bleScanTimeout)
			select {
			case deviceScanResult = <-ch:
				log.Printf("Found device: %s (RSSI: %d)", deviceScanResult.LocalName(), deviceScanResult.RSSI)
				cancelScan()
			case <-scanCtx.Done():
				adapter.StopScan()
				log.Println("Scan timed out or interrupted. Retrying...")
				cancelScan()
				time.Sleep(c.bleRetryDelay)
				continue
			}

			var device bluetooth.Device
			connectErrChan := make(chan error, 1)

			log.Printf("Connecting to %s...", deviceScanResult.Address.String())

			go func() {
				d, err := adapter.Connect(deviceScanResult.Address, bluetooth.ConnectionParams{})
				if err == nil {
					device = d
				}
				connectErrChan <- err
			}()

			select {
			case err := <-connectErrChan:
				if err != nil {
					log.Printf("Failed to connect: %v", err)
					c.publishConnection(false, 0)
					time.Sleep(c.bleRetryDelay)
					continue
				}
			case <-time.After(c.bleConnectTimeout):
				log.Println("Connection attempt timed out. Retrying...")
				adapter.StopScan()
				time.Sleep(c.bleRetryDelay)
				continue
			case <-ctx.Done():
				return
			}

			log.Printf("Connected to %s", deviceScanResult.LocalName())
			c.publishConnection(true, deviceScanResult.RSSI)

			discoverErrChan := make(chan error, 1)
			go func() {
				services, err := device.DiscoverServices([]bluetooth.UUID{c.bleServiceUUID})
				if err != nil || len(services) == 0 {
					discoverErrChan <- err
					return
				}

				chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{c.bleCharacteristicUUID})
				if err != nil || len(chars) == 0 {
					discoverErrChan <- err
					return
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
				discoverErrChan <- nil
			}()

			select {
			case err := <-discoverErrChan:
				if err != nil {
					log.Printf("Service discovery failed: %v", err)
					device.Disconnect()
					continue
				}
			case <-time.After(c.bleConnectTimeout):
				log.Println("Service discovery timed out. Disconnecting...")
				device.Disconnect()
				time.Sleep(c.bleRetryDelay)
				continue
			case <-ctx.Done():
				device.Disconnect()
				return
			}

			log.Println("BLEDOM device is ready.")

			heartbeatTicker := time.NewTicker(c.bleHeartbeatInterval)
			running := true
			heartbeatBuffer := make([]byte, 20)

			for running {
				select {
				case <-heartbeatTicker.C:
					if c.heartbeatChar.UUID() != (bluetooth.UUID{}) {
						_, err := c.heartbeatChar.Read(heartbeatBuffer)
						if err != nil {
							log.Printf("Heartbeat failed: %v", err)
							c.signalDisconnect()
						}
					}
				case <-c.disconnectChan:
					log.Println("Disconnection signal received. Resetting connection...")
					running = false

				case <-ctx.Done():
					log.Println("Disconnecting due to shutdown...")
					device.Disconnect()
					return
				}
			}

			heartbeatTicker.Stop()
			c.publishConnection(false, 0)

			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}

			if err := device.Disconnect(); err != nil {
				log.Printf("Disconnect warning: %v", err)
			}

			time.Sleep(c.bleRetryDelay)
		}
	}
}
