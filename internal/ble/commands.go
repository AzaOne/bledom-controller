package ble

import "time"

// SetPower builds and sends the power on/off command.
func (c *Controller) SetPower(isOn bool) {
	val := 0x00
	if isOn {
		val = 0x01
	}
	c.Write([]byte{0x7E, 0x04, 0x04, byte(val), 0x00, byte(val), 0xFF, 0x00, 0xEF})
}

// SetColor builds and sends the color command.
func (c *Controller) SetColor(r, g, b int) {
	c.Write([]byte{0x7E, 0x07, 0x05, 0x03, byte(r), byte(g), byte(b), 0x10, 0xEF})
}

// SetBrightness builds and sends the brightness command.
func (c *Controller) SetBrightness(val int) {
	c.Write([]byte{0x7E, 0x04, 0x01, byte(val), 0xFF, 0xFF, 0xFF, 0x00, 0xEF})
}

// SetSpeed builds and sends the effect speed command.
func (c *Controller) SetSpeed(val int) {
	if val < 0 {
		val = 0
	}
	if val > 100 {
		val = 100
	}
	c.Write([]byte{0x7E, 0x04, 0x02, byte(val), 0xFF, 0xFF, 0xFF, 0x00, 0xEF})
}

// SetHardwarePattern builds and sends the built-in pattern command.
func (c *Controller) SetHardwarePattern(id int) {
	c.Write([]byte{0x7E, 0x05, 0x03, byte(id + 128), 0x03, 0xFF, 0xFF, 0x00, 0xEF})
}

// SyncTime builds and sends the time synchronization command.
func (c *Controller) SyncTime() {
	now := time.Now()
	day := now.Weekday() // time.Sunday is 0, Monday is 1, etc.
	var deviceDay byte
	if day == time.Sunday {
		deviceDay = 6
	} else {
		deviceDay = byte(day - 1)
	}
	payload := []byte{
		0x7E, 0x07, 0x83,
		byte(now.Hour()),
		byte(now.Minute()),
		byte(now.Second()),
		deviceDay,
		0xFF, 0xEF,
	}
	c.Write(payload)
}

// SetRgbOrder builds and sends the RGB wire order command.
func (c *Controller) SetRgbOrder(v1, v2, v3 int) {
	payload := []byte{
		0x7E, 0x06, 0x81,
		byte(v1), byte(v2), byte(v3),
		0xFF, 0x00, 0xEF,
	}
	c.Write(payload)
}

// SetSchedule builds and sends the on-device schedule command.
func (c *Controller) SetSchedule(hour, minute, second int, weekdays byte, isOn, isSet bool) {
	var actionByte byte = 0x01 // 0x01 for OFF
	if isOn {
		actionByte = 0x00 // 0x00 for ON
	}

	var modeByte byte = 0x00 // 0x00 for CLEAR
	if isSet {
		modeByte = 0x80 // 0x80 for SET
	}

	payload := []byte{
		0x7E, 0x08, 0x82,
		byte(hour),
		byte(minute),
		byte(second),
		actionByte,
		modeByte | weekdays,
		0xEF,
	}
	c.Write(payload)
}
