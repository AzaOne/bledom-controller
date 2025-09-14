package ble

import (
    "context"
    "log"
    "time"

    "tinygo.org/x/bluetooth"
    "golang.org/x/time/rate" // New import
)

var (
    adapter = bluetooth.DefaultAdapter

    defaultServiceUUIDStr        = "0000fff0-0000-1000-8000-00805f9b34fb"
    defaultCharacteristicUUIDStr = "0000fff3-0000-1000-8000-00805f9b34fb"
)

// Controller manages the BLE connection and commands.
type Controller struct {
    characteristic bluetooth.DeviceCharacteristic
    // disconnectChan: An unbuffered channel used by commandWriterLoop or heartbeat to signal
    // a disconnection event to the main Run loop, which then handles reconnection.
    // It's created and closed by the Run loop itself for each connection session.
    disconnectChan chan struct{}
    // commandChan: A buffered channel to send BLE command payloads from multiple goroutines
    // to a single rate-limited writer goroutine.
    commandChan    chan []byte

    deviceNames           []string
    bleServiceUUID        bluetooth.UUID
    bleCharacteristicUUID bluetooth.UUID
    bleScanTimeout        time.Duration
    bleConnectTimeout     time.Duration
    bleHeartbeatInterval  time.Duration
    bleRetryDelay         time.Duration
    bleCommandLimiter     *rate.Limiter // New: Rate limiter for BLE commands
}

// NewController creates a new BLE controller with configurable parameters.
// It also starts a background goroutine for rate-limited command writing.
func NewController(ctx context.Context, deviceNames []string, scanTimeout, connectTimeout, heartbeatInterval, retryDelay time.Duration, commandRateLimitRate float64, commandRateLimitBurst int) *Controller {
    // Parse UUIDs. For now, using defaults, but could be passed as strings from config.
    serviceUUID, _ := bluetooth.ParseUUID(defaultServiceUUIDStr)
    characteristicUUID, _ := bluetooth.ParseUUID(defaultCharacteristicUUIDStr)

    // Log if UUIDs couldn't be parsed (shouldn't happen with valid hardcoded strings)
    if serviceUUID == (bluetooth.UUID{}) {
        log.Printf("Warning: Could not parse default BLE Service UUID: %s", defaultServiceUUIDStr)
    }
    if characteristicUUID == (bluetooth.UUID{}) {
        log.Printf("Warning: Could not parse default BLE Characteristic UUID: %s", defaultCharacteristicUUIDStr)
    }

    c := &Controller{
        deviceNames:           deviceNames,
        bleServiceUUID:        serviceUUID,
        bleCharacteristicUUID: characteristicUUID,
        bleScanTimeout:        scanTimeout,
        bleConnectTimeout:     connectTimeout,
        bleHeartbeatInterval:  heartbeatInterval,
        bleRetryDelay:         retryDelay,
        // The command channel buffer size allows some commands to queue up
        // while the rate limiter catches up, before dropping new commands.
        commandChan:           make(chan []byte, commandRateLimitBurst*2),
        bleCommandLimiter:     rate.NewLimiter(rate.Limit(commandRateLimitRate), commandRateLimitBurst),
    }

    go c.commandWriterLoop(ctx) // Start the dedicated command writer goroutine
    return c
}

// Write sends a byte command to the device through a rate-limited channel.
// This method is non-blocking. If the command queue is full, the command is dropped.
func (c *Controller) Write(payload []byte) {
    select {
    case c.commandChan <- payload:
        // Command successfully queued
    default:
        log.Printf("Warning: BLE command queue full, dropping command: %x", payload)
    }
}

// commandWriterLoop processes commands from commandChan, applying rate limits,
// and writes them to the BLE characteristic. This runs in its own goroutine.
func (c *Controller) commandWriterLoop(ctx context.Context) {
    log.Println("BLE command writer loop started.")
    for {
        select {
        case <-ctx.Done():
            log.Println("BLE command writer loop shutting down.")
            return
        case payload := <-c.commandChan:
            // Wait for a token from the rate limiter
            startWait := time.Now()
            if err := c.bleCommandLimiter.Wait(ctx); err != nil {
                // Context cancelled while waiting for token, e.g., during shutdown
                log.Printf("BLE command limiter wait interrupted: %v", err)
                return
            }
            if time.Since(startWait) > 50*time.Millisecond { // Log if command was significantly delayed
                log.Printf("Debug: BLE command \"%x\" delayed by rate limiter for %s", payload, time.Since(startWait))
            }

            if (c.characteristic == bluetooth.DeviceCharacteristic{}) {
                log.Printf("BLE device not connected. Dropping command: %x", payload)
                continue // Skip writing if not connected
            }

            _, err := c.characteristic.WriteWithoutResponse(payload)
            if err != nil {
                log.Printf("Failed to write to BLEDOM device (assuming disconnection): %v. Command: %x", err, payload)
                // Signal disconnection to the main Run loop if it's not already signalled.
                select {
                case c.disconnectChan <- struct{}{}:
                default:
                    // Channel full, means Run loop is already processing a disconnect or is not ready.
                }
            }
        }
    }
}

// contains is a helper function to check if a string is in a slice of strings
func contains(s []string, str string) bool {
    for _, v := range s {
        if v == str {
            return true
        }
    }
    return false
}

// Run starts the BLE connection management loop. It accepts a callback to report status.
func (c *Controller) Run(ctx context.Context, onStatusChange func(connected bool, rssi int16)) {
    onStatusChange(false, 0)

    for {
        select {
        case <-ctx.Done():
            log.Println("BLE controller shutting down.")
            return
        default:
            if err := adapter.Enable(); err != nil {
                log.Printf("Failed to enable adapter: %v", err)
                time.Sleep(c.bleRetryDelay)
                continue
            }

            log.Println("Scanning for BLEDOM device...")
            ch := make(chan bluetooth.ScanResult, 1)
            scanCtx, cancelScan := context.WithTimeout(ctx, c.bleScanTimeout)

            go adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
                if contains(c.deviceNames, result.LocalName()) {
                    adapter.StopScan()
                    select {
                    case ch <- result:
                    default:
                    }
                }
            })

            var deviceScanResult bluetooth.ScanResult
            select {
            case deviceScanResult = <-ch:
                log.Printf("Found device: %s (RSSI: %d)", deviceScanResult.LocalName(), deviceScanResult.RSSI)
            case <-scanCtx.Done():
                adapter.StopScan()
                log.Println("Scan timed out. Retrying...")
                cancelScan() // Ensure context is canceled
                time.Sleep(c.bleRetryDelay)
                continue
            case <-ctx.Done():
                adapter.StopScan()
                log.Println("BLE scan interrupted by shutdown signal.")
                cancelScan() // Ensure context is canceled
                return
            }
            cancelScan() // Scan finished, cancel scan context

            device, err := adapter.Connect(deviceScanResult.Address, bluetooth.ConnectionParams{})
            if err != nil {
                log.Printf("Failed to connect: %v", err)
                onStatusChange(false, 0)
                time.Sleep(c.bleRetryDelay)
                continue
            }
            log.Printf("Connected to %s", deviceScanResult.LocalName())
            onStatusChange(true, deviceScanResult.RSSI)

            discoveryCtx, cancelDiscovery := context.WithTimeout(context.Background(), c.bleConnectTimeout)
            defer cancelDiscovery()

            services, err := device.DiscoverServices([]bluetooth.UUID{c.bleServiceUUID})
            if err != nil || len(services) == 0 || discoveryCtx.Err() != nil {
                log.Printf("Failed to discover BLEDOM services: %v", err)
                device.Disconnect()
                continue
            }
            
            chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{c.bleCharacteristicUUID})
            if err != nil || len(chars) == 0 || discoveryCtx.Err() != nil {
                log.Printf("Failed to discover BLEDOM characteristics: %v", err)
                device.Disconnect()
                continue
            }
            c.characteristic = chars[0]
            
            log.Println("BLEDOM device is ready.")
            // Initialize disconnectChan for this connection session.
            // This unbuffered channel is used to signal a disconnect from other goroutines.
            c.disconnectChan = make(chan struct{}) 
            
            heartbeatTicker := time.NewTicker(c.bleHeartbeatInterval)
            defer heartbeatTicker.Stop()

            running := true
            heartbeatBuffer := make([]byte, 8) 

            for running {
                select {
                case <-heartbeatTicker.C:
                    _, err := c.characteristic.Read(heartbeatBuffer)
                    if err != nil {
                        log.Printf("Heartbeat read failed on main characteristic (assuming disconnection): %v", err)
                        // Signal disconnection to the Run loop.
                        select {
                        case c.disconnectChan <- struct{}{}:
                        default:
                            // Channel full, means Run loop is already processing a disconnect.
                        }
                    } else {
                        // log.Println("Heartbeat check successful, connection is active.") // Can be noisy
                    }
                case <-c.disconnectChan:
                    // Received a signal from commandWriterLoop or heartbeat that device disconnected
                    log.Println("Disconnection signal received. Rescanning...")
                    running = false 

                case <-ctx.Done():
                    log.Println("Disconnecting from device due to shutdown...")
                    device.Disconnect()
                    return 
                }
            }

            onStatusChange(false, 0)
            c.characteristic = bluetooth.DeviceCharacteristic{}
            close(c.disconnectChan) // Close the channel as this connection session ends
            c.disconnectChan = nil // Reset for next connection
            device.Disconnect()
            time.Sleep(c.bleRetryDelay)
        }
    }
}
