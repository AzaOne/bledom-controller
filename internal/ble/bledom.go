package ble

import (
    "context"
    "log"
    "time"

    "tinygo.org/x/bluetooth"
)

var (
    adapter = bluetooth.DefaultAdapter

    defaultServiceUUIDStr        = "0000fff0-0000-1000-8000-00805f9b34fb"
    defaultCharacteristicUUIDStr = "0000fff3-0000-1000-8000-00805f9b34fb"
)

// Controller manages the BLE connection and commands.
type Controller struct {
    characteristic bluetooth.DeviceCharacteristic
    disconnectChan chan struct{}

    deviceNames           []string
    bleServiceUUID        bluetooth.UUID
    bleCharacteristicUUID bluetooth.UUID
    bleScanTimeout        time.Duration
    bleConnectTimeout     time.Duration
    bleHeartbeatInterval  time.Duration
    bleRetryDelay         time.Duration
}

// NewController creates a new BLE controller with configurable parameters.
func NewController(deviceNames []string, scanTimeout, connectTimeout, heartbeatInterval, retryDelay time.Duration) *Controller {
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

        return &Controller{
            deviceNames: deviceNames,
            bleServiceUUID: serviceUUID,
            bleCharacteristicUUID: characteristicUUID,
            bleScanTimeout: scanTimeout,
            bleConnectTimeout: connectTimeout,
            bleHeartbeatInterval: heartbeatInterval,
            bleRetryDelay: retryDelay,
        }
}

// Write sends a byte command to the device.
func (c *Controller) Write(payload []byte) {
    if (c.characteristic == bluetooth.DeviceCharacteristic{}) {
        log.Println("BLE device not connected. Ignoring command.")
        return
    }
    _, err := c.characteristic.WriteWithoutResponse(payload)
    if err != nil {
        log.Printf("Failed to write to BLEDOM device (assuming disconnection): %v", err)
        select {
        case <-c.disconnectChan:
        default:
            close(c.disconnectChan)
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
            // Note: defer cancelDiscovery() should be here, not below the loop,
            // to ensure it cleans up if the loop `continue`s or `return`s.
            // It was fine before because the loop `continue`d/`return`ed only after `cancelDiscovery()` was called
            // OR if device.Disconnect() was called, but better to be explicit outside the loop.
            // Let's ensure it's called on every exit path from this block.

            services, err := device.DiscoverServices([]bluetooth.UUID{c.bleServiceUUID})
            if err != nil || len(services) == 0 || discoveryCtx.Err() != nil {
                log.Printf("Failed to discover BLEDOM services: %v", err)
                cancelDiscovery() // Make sure this is called
                device.Disconnect()
                continue
            }
            
            chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{c.bleCharacteristicUUID})
            if err != nil || len(chars) == 0 || discoveryCtx.Err() != nil {
                log.Printf("Failed to discover BLEDOM characteristics: %v", err)
                cancelDiscovery() // Make sure this is called
                device.Disconnect()
                continue
            }
            c.characteristic = chars[0]
            cancelDiscovery() // Discovery finished successfully

            log.Println("BLEDOM device is ready.")
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
                        close(c.disconnectChan)
                    } else {
                        // log.Println("Heartbeat check successful, connection is active.") // Can be noisy
                    }

                case <-c.disconnectChan:
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
            c.disconnectChan = nil
            device.Disconnect()
            time.Sleep(c.bleRetryDelay)
        }
    }
}
