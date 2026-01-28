package ble

import (
	"context"
	"log"
	"time"

	"golang.org/x/time/rate"
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
	heartbeatChar  bluetooth.DeviceCharacteristic
	
	// disconnectChan: Використовується для сигналізації про розрив з'єднання.
	// Створюється один раз і є буферизованим.
	disconnectChan chan struct{}
	
	commandChan chan []byte

	deviceNames           []string
	bleServiceUUID        bluetooth.UUID
	bleCharacteristicUUID bluetooth.UUID
	bleScanTimeout        time.Duration
	bleConnectTimeout     time.Duration
	bleHeartbeatInterval  time.Duration
	bleRetryDelay         time.Duration
	bleCommandLimiter     *rate.Limiter
}

// NewController creates a new BLE controller.
func NewController(ctx context.Context, deviceNames []string, scanTimeout, connectTimeout, heartbeatInterval, retryDelay time.Duration, commandRateLimitRate float64, commandRateLimitBurst int) *Controller {
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
		// Буферизований канал (1), щоб не блокувати відправника
		disconnectChan:        make(chan struct{}, 1), 
		bleCommandLimiter:     rate.NewLimiter(rate.Limit(commandRateLimitRate), commandRateLimitBurst),
	}

	go c.commandWriterLoop(ctx)
	return c
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

			// ВИПРАВЛЕННЯ ТУТ: Додано дужки до UUID(), бо це метод
			if c.characteristic.UUID() == (bluetooth.UUID{}) {
				// Пристрій не підключено або характеристика не ініціалізована, ігноруємо команду
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
		// Вже є сигнал в каналі або ніхто не слухає, це нормально
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

// Run starts the BLE connection management loop.
func (c *Controller) Run(ctx context.Context, onStatusChange func(connected bool, rssi int16)) {
	onStatusChange(false, 0)

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

			// Очищаємо канал роз'єднання перед новим циклом
			select {
			case <-c.disconnectChan:
			default:
			}

			// Скидаємо характеристики
			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}

			log.Println("Scanning for BLEDOM device...")
			
			// Примусово зупиняємо попередній скан, якщо він завис
			adapter.StopScan() 

			ch := make(chan bluetooth.ScanResult, 1)
			
			// Запуск сканування
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
			
			// Очікування результатів сканування
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

			// 2. Connect (With Timeout Wrapper)
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
					onStatusChange(false, 0)
					time.Sleep(c.bleRetryDelay)
					continue
				}
			case <-time.After(c.bleConnectTimeout):
				log.Println("Connection attempt timed out (BlueZ stuck?). Retrying...")
				adapter.StopScan() 
				time.Sleep(c.bleRetryDelay)
				continue
			case <-ctx.Done():
				return
			}

			log.Printf("Connected to %s", deviceScanResult.LocalName())
			onStatusChange(true, deviceScanResult.RSSI)

			// 3. Discover Services (With Timeout Wrapper)
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

				// Heartbeat discovery (optional)
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

			// 4. Heartbeat Loop
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
			onStatusChange(false, 0)
			
			c.characteristic = bluetooth.DeviceCharacteristic{}
			c.heartbeatChar = bluetooth.DeviceCharacteristic{}
			
			if err := device.Disconnect(); err != nil {
				log.Printf("Disconnect warning: %v", err)
			}
			
			time.Sleep(c.bleRetryDelay)
		}
	}
}
