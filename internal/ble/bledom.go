package ble

import (
	"context"
	"log"
	"time"

	"tinygo.org/x/bluetooth"
)

var (
	adapter               = bluetooth.DefaultAdapter
	serviceUUIDStr        = "0000fff0-0000-1000-8000-00805f9b34fb"
	characteristicUUIDStr = "0000fff3-0000-1000-8000-00805f9b34fb"
)

// Controller manages the BLE connection and commands.
type Controller struct {
    characteristic bluetooth.DeviceCharacteristic
    disconnectChan chan struct{}
}

// NewController creates a new BLE controller.
func NewController() *Controller {
	return &Controller{}
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

// Run starts the BLE connection management loop. It accepts a callback to report status.
func (c *Controller) Run(ctx context.Context, onStatusChange func(connected bool, rssi int16)) {
	serviceUUID, _ := bluetooth.ParseUUID(serviceUUIDStr)
	characteristicUUID, _ := bluetooth.ParseUUID(characteristicUUIDStr)

	onStatusChange(false, 0)

	for {
		select {
		case <-ctx.Done():
			log.Println("BLE controller shutting down.")
			return
		default:
			if err := adapter.Enable(); err != nil {
				log.Printf("Failed to enable adapter: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			log.Println("Scanning for BLEDOM device...")
			ch := make(chan bluetooth.ScanResult, 1)
			scanCtx, cancelScan := context.WithTimeout(ctx, 30*time.Second)

			go adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
				if result.LocalName() == "ELK-BLEDOM   " || result.LocalName() == "BLEDOM" {
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
				cancelScan()
				continue
			case <-ctx.Done():
				adapter.StopScan()
				log.Println("BLE scan interrupted by shutdown signal.")
				cancelScan()
				return 
			}
			cancelScan()

			device, err := adapter.Connect(deviceScanResult.Address, bluetooth.ConnectionParams{})
			if err != nil {
				log.Printf("Failed to connect: %v", err)
				onStatusChange(false, 0)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Printf("Connected to %s", deviceScanResult.LocalName())
			onStatusChange(true, deviceScanResult.RSSI)

			discoveryCtx, cancelDiscovery := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancelDiscovery()

			services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
			if err != nil || len(services) == 0 || discoveryCtx.Err() != nil {
				log.Printf("Failed to discover BLEDOM services: %v", err)
				device.Disconnect()
				continue
			}
			
			chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{characteristicUUID})
			if err != nil || len(chars) == 0 || discoveryCtx.Err() != nil {
				log.Printf("Failed to discover BLEDOM characteristics: %v", err)
				device.Disconnect()
				continue
			}
			c.characteristic = chars[0]
			cancelDiscovery()

			log.Println("BLEDOM device is ready.")
			c.disconnectChan = make(chan struct{})
			
			heartbeatTicker := time.NewTicker(60 * time.Second)
			defer heartbeatTicker.Stop()

			running := true
			// This buffer is for the read attempt.
			heartbeatBuffer := make([]byte, 8) 

			for running {
				select {
				case <-heartbeatTicker.C:
					// This is a reliable way to check if the connection is still alive.
					// Don't care about the data, only if the operation succeeds or fails.
					_, err := c.characteristic.Read(heartbeatBuffer)
					if err != nil {
						log.Printf("Heartbeat read failed on main characteristic (assuming disconnection): %v", err)
						close(c.disconnectChan)
					} else {
						log.Println("Heartbeat check successful, connection is active.")
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
		}
	}
}
