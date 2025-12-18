//go:build linux && arm

// Package epd provides a SPI-based driver for the Waveshare 12.48" tri-color
// e-paper (B) panel, implemented in pure Go using periph.io instead of the
// original C SDK. This file focuses on the low-level DEV_* layer (GPIO/SPI)
// and basic wiring; the higher-level EPD_12in48B_* sequences are ported
// progressively from the C reference.
package epd

import (
	"context"
	"fmt"
	"time"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

// NOTE: These BCM pin numbers are taken from the Waveshare DEV_Config.h
// you provided. We use periph.io's gpioreg.ByName("GPIO<n>") to map them.

// SPI pins (handled by /dev/spidev, names kept for documentation)
const (
	bcmEPDSCK  = 11
	bcmEPDMOSI = 10
)

// Chip select pins for the four panel segments.
const (
	bcmEPDM1CS = 8
	bcmEPDS1CS = 7
	bcmEPDM2CS = 17
	bcmEPDS2CS = 18
)

// Data/command (DC) pins for the two half-panels.
const (
	bcmEPDM1S1DC = 13
	bcmEPDM2S2DC = 22
)

// Reset pins.
const (
	bcmEPDM1S1RST = 6
	bcmEPDM2S2RST = 23
)

// Busy pins.
const (
	bcmEPDM1BUSY = 5
	bcmEPDS1BUSY = 19
	bcmEPDM2BUSY = 27
	bcmEPDS2BUSY = 24
)

// Dev is the Go equivalent of the C DEV_* layer. It wraps the SPI bus and
// all GPIO pins required by the 12.48" B panel.
type Dev struct {
	spi spi.Conn

	// Chip selects
	m1CS gpio.PinOut
	s1CS gpio.PinOut
	m2CS gpio.PinOut
	s2CS gpio.PinOut

	// Data/command
	m1s1DC gpio.PinOut
	m2s2DC gpio.PinOut

	// Reset
	m1s1RST gpio.PinOut
	m2s2RST gpio.PinOut

	// Busy inputs
	m1BUSY gpio.PinIn
	s1BUSY gpio.PinIn
	m2BUSY gpio.PinIn
	s2BUSY gpio.PinIn
}

// Driver is the high-level handle used by the rest of the application.
// It owns a Dev and will later expose EPD_12in48B_* style operations.
type Driver struct {
	dev *Dev
}

// defaultDriver is a package-level singleton used by simple wrappers if needed.
var defaultDriver *Driver

// Init initializes periph.io, opens the SPI bus, configures all GPIO pins,
// and returns a ready-to-use Driver.
//
// It is the pure-Go equivalent of DEV_ModuleInit + parts of EPD_12in48B_Init
// that deal with basic GPIO/SPI setup (but not the panel register sequences).
func Init(ctx context.Context) (*Driver, error) {
	// Initialize periph.io host.
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("epd: periph host init failed: %w", err)
	}

	// Open the default SPI port. On Raspberry Pi this is typically /dev/spidev0.0.
	port, err := spireg.Open("")
	if err != nil {
		return nil, fmt.Errorf("epd: failed to open SPI port: %w", err)
	}

	// Connect with a conservative frequency and mode 0.
	// The exact maxHz can be tuned later based on C SDK defaults.
	const maxHz = 2_000_000 // 2MHz
	spiConn, err := port.Connect(maxHz, spi.Mode0, 8)
	if err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("epd: failed to connect SPI: %w", err)
	}

	// Helper to resolve a BCM GPIO number via periph.
	mustGPIOOut := func(num int, initialLevel gpio.Level) gpio.PinOut {
		name := fmt.Sprintf("GPIO%d", num)
		p := gpioreg.ByName(name)
		if p == nil {
			panic(fmt.Sprintf("epd: gpio %s not found", name))
		}
		if err := p.Out(initialLevel); err != nil {
			panic(fmt.Sprintf("epd: gpio %s Out failed: %v", name, err))
		}
		return p
	}
	mustGPIOIn := func(num int) gpio.PinIn {
		name := fmt.Sprintf("GPIO%d", num)
		p := gpioreg.ByName(name)
		if p == nil {
			panic(fmt.Sprintf("epd: gpio %s not found", name))
		}
		if err := p.In(gpio.PullUp, gpio.NoEdge); err != nil {
			panic(fmt.Sprintf("epd: gpio %s In failed: %v", name, err))
		}
		return p
	}

	dev := &Dev{
		spi:     spiConn,
		m1CS:    mustGPIOOut(bcmEPDM1CS, gpio.High),
		s1CS:    mustGPIOOut(bcmEPDS1CS, gpio.High),
		m2CS:    mustGPIOOut(bcmEPDM2CS, gpio.High),
		s2CS:    mustGPIOOut(bcmEPDS2CS, gpio.High),
		m1s1DC:  mustGPIOOut(bcmEPDM1S1DC, gpio.Low),
		m2s2DC:  mustGPIOOut(bcmEPDM2S2DC, gpio.Low),
		m1s1RST: mustGPIOOut(bcmEPDM1S1RST, gpio.High),
		m2s2RST: mustGPIOOut(bcmEPDM2S2RST, gpio.High),
		m1BUSY:  mustGPIOIn(bcmEPDM1BUSY),
		s1BUSY:  mustGPIOIn(bcmEPDS1BUSY),
		m2BUSY:  mustGPIOIn(bcmEPDM2BUSY),
		s2BUSY:  mustGPIOIn(bcmEPDS2BUSY),
	}

	driver := &Driver{dev: dev}
	defaultDriver = driver
	return driver, nil
}

// Close releases SPI and unexports pins. It is the rough equivalent of
// DEV_ModuleExit in the C layer.
func (d *Driver) Close() error {
	// periph.io pins don't need explicit close, but the SPI port does.
	// However, spi.Conn does not expose Close directly; the underlying
	// port may implement io.Closer via type assertion.
	if closer, ok := d.dev.spi.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// --- DEV_* equivalents (low-level helpers) ---

// digitalWrite is a small helper mirroring DEV_Digital_Write.
func digitalWrite(pin gpio.PinOut, value bool) {
	if value {
		_ = pin.Out(gpio.High)
	} else {
		_ = pin.Out(gpio.Low)
	}
}

// digitalRead mirrors DEV_Digital_Read.
func digitalRead(pin gpio.PinIn) bool {
	return pin.Read() == gpio.High
}

// delayUs / delayMs mirror DEV_Delay_us / DEV_Delay_ms.
func delayUs(us uint16) {
	time.Sleep(time.Duration(us) * time.Microsecond)
}

func delayMs(ms uint32) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// spiWriteByte mirrors DEV_SPI_WriteByte.
func (d *Dev) spiWriteByte(b byte) error {
	// periph.io requires a write+read buffer; RX can be nil when not needed.
	tx := []byte{b}
	return d.spi.Tx(tx, nil)
}

// spiReadByte mirrors DEV_SPI_ReadByte(Reg).
// In the original C code it is used as "temp = DEV_SPI_ReadByte(0x00);",
// i.e. sending a dummy byte and reading one back.
func (d *Dev) spiReadByte(dummy byte) (byte, error) {
	tx := []byte{dummy}
	rx := make([]byte, 1)
	if err := d.spi.Tx(tx, rx); err != nil {
		return 0, err
	}
	return rx[0], nil
}

// --- High-level wiring helpers & EPD sequence port (from C reference) ---

// reset is a Go port of EPD_Reset() from the C code.
func (d *Driver) reset() {
	// EPD_Reset:
	// DEV_Digital_Write(EPD_M1S1_RST_PIN, 1);
	// DEV_Digital_Write(EPD_M2S2_RST_PIN, 1);
	// DEV_Delay_ms(200);
	// DEV_Digital_Write(EPD_M1S1_RST_PIN, 0);
	// DEV_Digital_Write(EPD_M2S2_RST_PIN, 0);
	// DEV_Delay_ms(10);
	// DEV_Digital_Write(EPD_M1S1_RST_PIN, 1);
	// DEV_Digital_Write(EPD_M2S2_RST_PIN, 1);
	// DEV_Delay_ms(200);

	digitalWrite(d.dev.m1s1RST, true)
	digitalWrite(d.dev.m2s2RST, true)
	delayMs(200)

	digitalWrite(d.dev.m1s1RST, false)
	digitalWrite(d.dev.m2s2RST, false)
	delayMs(10)

	digitalWrite(d.dev.m1s1RST, true)
	digitalWrite(d.dev.m2s2RST, true)
	delayMs(200)
}

// The following helpers are Go ports of EPD_M1/S1/M2/S2_SendCommand/SendData
// and EPD_M1M2_SendCommand / EPD_M1S1M2S2_SendCommand/Data. These will be
// used by the eventual EPD_12in48B_Init/Clear/Display/Sleep ports.

// M1 commands/data.
func (d *Driver) m1SendCommand(reg byte) error {
	digitalWrite(d.dev.m1s1DC, false)
	digitalWrite(d.dev.m1CS, false)
	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.m1CS, true)
		return err
	}
	digitalWrite(d.dev.m1CS, true)
	return nil
}

func (d *Driver) m1SendData(data byte) error {
	digitalWrite(d.dev.m1s1DC, true)
	digitalWrite(d.dev.m1CS, false)
	if err := d.dev.spiWriteByte(data); err != nil {
		digitalWrite(d.dev.m1CS, true)
		return err
	}
	digitalWrite(d.dev.m1CS, true)
	return nil
}

// S1 commands/data.
func (d *Driver) s1SendCommand(reg byte) error {
	digitalWrite(d.dev.m1s1DC, false)
	digitalWrite(d.dev.s1CS, false)
	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.s1CS, true)
		return err
	}
	digitalWrite(d.dev.s1CS, true)
	return nil
}

func (d *Driver) s1SendData(data byte) error {
	digitalWrite(d.dev.m1s1DC, true)
	digitalWrite(d.dev.s1CS, false)
	if err := d.dev.spiWriteByte(data); err != nil {
		digitalWrite(d.dev.s1CS, true)
		return err
	}
	digitalWrite(d.dev.s1CS, true)
	return nil
}

// M2 commands/data.
func (d *Driver) m2SendCommand(reg byte) error {
	digitalWrite(d.dev.m2s2DC, false)
	digitalWrite(d.dev.m2CS, false)
	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.m2CS, true)
		return err
	}
	digitalWrite(d.dev.m2CS, true)
	return nil
}

func (d *Driver) m2SendData(data byte) error {
	digitalWrite(d.dev.m2s2DC, true)
	digitalWrite(d.dev.m2CS, false)
	if err := d.dev.spiWriteByte(data); err != nil {
		digitalWrite(d.dev.m2CS, true)
		return err
	}
	digitalWrite(d.dev.m2CS, true)
	return nil
}

// S2 commands/data.
func (d *Driver) s2SendCommand(reg byte) error {
	digitalWrite(d.dev.m2s2DC, false)
	digitalWrite(d.dev.s2CS, false)
	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.s2CS, true)
		return err
	}
	digitalWrite(d.dev.s2CS, true)
	return nil
}

func (d *Driver) s2SendData(data byte) error {
	digitalWrite(d.dev.m2s2DC, true)
	digitalWrite(d.dev.s2CS, false)
	if err := d.dev.spiWriteByte(data); err != nil {
		digitalWrite(d.dev.s2CS, true)
		return err
	}
	digitalWrite(d.dev.s2CS, true)
	return nil
}

// m1m2SendCommand mirrors EPD_M1M2_SendCommand.
func (d *Driver) m1m2SendCommand(reg byte) error {
	digitalWrite(d.dev.m1s1DC, false)
	digitalWrite(d.dev.m2s2DC, false)

	digitalWrite(d.dev.m1CS, false)
	digitalWrite(d.dev.m2CS, false)
	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.m1CS, true)
		digitalWrite(d.dev.m2CS, true)
		return err
	}
	digitalWrite(d.dev.m1CS, true)
	digitalWrite(d.dev.m2CS, true)
	return nil
}

// m1s1m2s2SendCommand mirrors EPD_M1S1M2S2_SendCommand.
func (d *Driver) m1s1m2s2SendCommand(reg byte) error {
	digitalWrite(d.dev.m1s1DC, false)
	digitalWrite(d.dev.m2s2DC, false)

	digitalWrite(d.dev.m1CS, false)
	digitalWrite(d.dev.s1CS, false)
	digitalWrite(d.dev.m2CS, false)
	digitalWrite(d.dev.s2CS, false)

	if err := d.dev.spiWriteByte(reg); err != nil {
		digitalWrite(d.dev.m1CS, true)
		digitalWrite(d.dev.s1CS, true)
		digitalWrite(d.dev.m2CS, true)
		digitalWrite(d.dev.s2CS, true)
		return err
	}

	digitalWrite(d.dev.m1CS, true)
	digitalWrite(d.dev.s1CS, true)
	digitalWrite(d.dev.m2CS, true)
	digitalWrite(d.dev.s2CS, true)
	return nil
}

// m1s1m2s2SendData mirrors EPD_M1S1M2S2_SendData.
func (d *Driver) m1s1m2s2SendData(data byte) error {
	digitalWrite(d.dev.m1s1DC, true)
	digitalWrite(d.dev.m2s2DC, true)

	digitalWrite(d.dev.m1CS, false)
	digitalWrite(d.dev.s1CS, false)
	digitalWrite(d.dev.m2CS, false)
	digitalWrite(d.dev.s2CS, false)

	if err := d.dev.spiWriteByte(data); err != nil {
		digitalWrite(d.dev.m1CS, true)
		digitalWrite(d.dev.s1CS, true)
		digitalWrite(d.dev.m2CS, true)
		digitalWrite(d.dev.s2CS, true)
		return err
	}

	digitalWrite(d.dev.m1CS, true)
	digitalWrite(d.dev.s1CS, true)
	digitalWrite(d.dev.m2CS, true)
	digitalWrite(d.dev.s2CS, true)
	return nil
}

//
// Busy-wait helpers (ports of EPD_M1/2/S1/2_ReadBusy from C)
//

func (d *Driver) m1ReadBusy() {
	// C code logic (intended):
	//
	//	do {
	//	  EPD_M1_SendCommand(0x71);
	//	  busy = DEV_Digital_Read(EPD_M1_BUSY_PIN);
	//	  busy = !(busy & 0x01);
	//	} while (busy);
	//
	// Here: BUSY=0 means busy, 1 means ready.
	for {
		_ = d.m1SendCommand(0x71)
		if digitalRead(d.dev.m1BUSY) {
			break
		}
	}
	delayMs(200)
}

func (d *Driver) m2ReadBusy() {
	for {
		_ = d.m2SendCommand(0x71)
		if digitalRead(d.dev.m2BUSY) {
			break
		}
	}
	delayMs(200)
}

func (d *Driver) s1ReadBusy() {
	for {
		_ = d.s1SendCommand(0x71)
		if digitalRead(d.dev.s1BUSY) {
			break
		}
	}
	delayMs(200)
}

func (d *Driver) s2ReadBusy() {
	for {
		_ = d.s2SendCommand(0x71)
		if digitalRead(d.dev.s2BUSY) {
			break
		}
	}
	delayMs(200)
}

//
// LUT tables & EPD_SetLut port
//

var lutVCOM1 = [60]byte{
	0x00, 0x10, 0x10, 0x01, 0x08, 0x01,
	0x00, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x00, 0x08, 0x01, 0x08, 0x01, 0x06,
	0x00, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x06,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x01,
	0x00, 0x04, 0x05, 0x08, 0x08, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var lutWW1 = [60]byte{
	0x91, 0x10, 0x10, 0x01, 0x08, 0x01,
	0x04, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x84, 0x08, 0x01, 0x08, 0x01, 0x06,
	0x80, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x06,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x01,
	0x08, 0x04, 0x05, 0x08, 0x08, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var lutBW1 = [60]byte{
	0xA8, 0x10, 0x10, 0x01, 0x08, 0x01,
	0x84, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x84, 0x08, 0x01, 0x08, 0x01, 0x06,
	0x86, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x8C, 0x05, 0x01, 0x1E, 0x0F, 0x06,
	0x8C, 0x05, 0x01, 0x1E, 0x0F, 0x01,
	0xF0, 0x04, 0x05, 0x08, 0x08, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var lutWB1 = [60]byte{
	0x91, 0x10, 0x10, 0x01, 0x08, 0x01,
	0x04, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x84, 0x08, 0x01, 0x08, 0x01, 0x06,
	0x80, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x06,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x01,
	0x08, 0x04, 0x05, 0x08, 0x08, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var lutBB1 = [60]byte{
	0x92, 0x10, 0x10, 0x01, 0x08, 0x01,
	0x80, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x84, 0x08, 0x01, 0x08, 0x01, 0x06,
	0x04, 0x06, 0x01, 0x06, 0x01, 0x05,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x06,
	0x00, 0x05, 0x01, 0x1E, 0x0F, 0x01,
	0x01, 0x04, 0x05, 0x08, 0x08, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// setLUT is a Go port of EPD_SetLut() from the C code.
func (d *Driver) setLUT() error {
	// vcom
	if err := d.m1s1m2s2SendCommand(0x20); err != nil {
		return err
	}
	for _, v := range lutVCOM1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	// 0x21 - red not used (ww)
	if err := d.m1s1m2s2SendCommand(0x21); err != nil {
		return err
	}
	for _, v := range lutWW1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	// 0x22 - bw
	if err := d.m1s1m2s2SendCommand(0x22); err != nil {
		return err
	}
	for _, v := range lutBW1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	// 0x23 - wb
	if err := d.m1s1m2s2SendCommand(0x23); err != nil {
		return err
	}
	for _, v := range lutWB1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	// 0x24 - bb
	if err := d.m1s1m2s2SendCommand(0x24); err != nil {
		return err
	}
	for _, v := range lutBB1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	// 0x25 - bb (reusing ww1 in C code)
	if err := d.m1s1m2s2SendCommand(0x25); err != nil {
		return err
	}
	for _, v := range lutWW1 {
		if err := d.m1s1m2s2SendData(v); err != nil {
			return err
		}
	}
	return nil
}

//
// High-level EPD operations: Init / Clear / Display / TurnOn / Sleep
//

// panelVersion corresponds to the "Version" global in the C code.
// For now we default to 1 (original sequence).
const panelVersion = 1

// InitPanel is the Go equivalent of EPD_12in48B_Init(void).
func (d *Driver) InitPanel() error {
	// DEV_Digital_Write(EPD_M1_CS_PIN, 1) ... etc.
	digitalWrite(d.dev.m1CS, true)
	digitalWrite(d.dev.s1CS, true)
	digitalWrite(d.dev.m2CS, true)
	digitalWrite(d.dev.s2CS, true)

	// EPD_Reset();
	d.reset()

	switch panelVersion {
	case 1:
		// Panel setting
		if err := d.m1SendCommand(0x00); err != nil {
			return err
		}
		if err := d.m1SendData(0x2f); err != nil {
			return err
		}
		if err := d.s1SendCommand(0x00); err != nil {
			return err
		}
		if err := d.s1SendData(0x2f); err != nil {
			return err
		}
		if err := d.m2SendCommand(0x00); err != nil {
			return err
		}
		if err := d.m2SendData(0x23); err != nil {
			return err
		}
		if err := d.s2SendCommand(0x00); err != nil {
			return err
		}
		if err := d.s2SendData(0x23); err != nil {
			return err
		}

		// POWER SETTING (M1/M2)
		if err := d.m1SendCommand(0x01); err != nil {
			return err
		}
		for _, v := range []byte{0x07, 0x17, 0x3F, 0x3F, 0x0d} {
			if err := d.m1SendData(v); err != nil {
				return err
			}
		}
		if err := d.m2SendCommand(0x01); err != nil {
			return err
		}
		for _, v := range []byte{0x07, 0x17, 0x3F, 0x3F, 0x0d} {
			if err := d.m2SendData(v); err != nil {
				return err
			}
		}

		// booster soft start
		if err := d.m1SendCommand(0x06); err != nil {
			return err
		}
		for _, v := range []byte{0x17, 0x17, 0x39, 0x17} {
			if err := d.m1SendData(v); err != nil {
				return err
			}
		}
		if err := d.m2SendCommand(0x06); err != nil {
			return err
		}
		for _, v := range []byte{0x17, 0x17, 0x39, 0x17} {
			if err := d.m2SendData(v); err != nil {
				return err
			}
		}

		// resolution setting
		if err := d.m1SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x88, 0x01, 0xEC} {
			if err := d.m1SendData(v); err != nil {
				return err
			}
		}
		if err := d.s1SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x90, 0x01, 0xEC} {
			if err := d.s1SendData(v); err != nil {
				return err
			}
		}
		if err := d.m2SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x90, 0x01, 0xEC} {
			if err := d.m2SendData(v); err != nil {
				return err
			}
		}
		if err := d.s2SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x88, 0x01, 0xEC} {
			if err := d.s2SendData(v); err != nil {
				return err
			}
		}

		// DUSPI
		if err := d.m1s1m2s2SendCommand(0x15); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x20); err != nil {
			return err
		}

		// PLL
		if err := d.m1s1m2s2SendCommand(0x30); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x08); err != nil {
			return err
		}

		// Vcom and data interval
		if err := d.m1s1m2s2SendCommand(0x50); err != nil {
			return err
		}
		for _, v := range []byte{0x31, 0x07} {
			if err := d.m1s1m2s2SendData(v); err != nil {
				return err
			}
		}

		// TCON
		if err := d.m1s1m2s2SendCommand(0x60); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x22); err != nil {
			return err
		}

		// POWER SETTING (E0)
		if err := d.m1SendCommand(0xE0); err != nil {
			return err
		}
		if err := d.m1SendData(0x01); err != nil {
			return err
		}
		if err := d.m2SendCommand(0xE0); err != nil {
			return err
		}
		if err := d.m2SendData(0x01); err != nil {
			return err
		}

		if err := d.m1s1m2s2SendCommand(0xE3); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x00); err != nil {
			return err
		}

		if err := d.m1SendCommand(0x82); err != nil {
			return err
		}
		if err := d.m1SendData(0x1c); err != nil {
			return err
		}
		if err := d.m2SendCommand(0x82); err != nil {
			return err
		}
		if err := d.m2SendData(0x1c); err != nil {
			return err
		}

		// LUT
		if err := d.setLUT(); err != nil {
			return err
		}

	case 2:
		// Version 2 panel setting (Display mode, as in C code)
		if err := d.m1SendCommand(0x00); err != nil {
			return err
		}
		if err := d.m1SendData(0x0f); err != nil {
			return err
		}
		if err := d.s1SendCommand(0x00); err != nil {
			return err
		}
		if err := d.s1SendData(0x0f); err != nil {
			return err
		}
		if err := d.m2SendCommand(0x00); err != nil {
			return err
		}
		if err := d.m2SendData(0x03); err != nil {
			return err
		}
		if err := d.s2SendCommand(0x00); err != nil {
			return err
		}
		if err := d.s2SendData(0x03); err != nil {
			return err
		}

		// booster soft start
		if err := d.m1SendCommand(0x06); err != nil {
			return err
		}
		for _, v := range []byte{0x17, 0x17, 0x39, 0x17} {
			if err := d.m1SendData(v); err != nil {
				return err
			}
		}
		if err := d.m2SendCommand(0x06); err != nil {
			return err
		}
		for _, v := range []byte{0x17, 0x17, 0x39, 0x17} {
			if err := d.m2SendData(v); err != nil {
				return err
			}
		}

		// resolution setting (same as Version 1)
		if err := d.m1SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x88, 0x01, 0xEC} {
			if err := d.m1SendData(v); err != nil {
				return err
			}
		}
		if err := d.s1SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x90, 0x01, 0xEC} {
			if err := d.s1SendData(v); err != nil {
				return err
			}
		}
		if err := d.m2SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x90, 0x01, 0xEC} {
			if err := d.m2SendData(v); err != nil {
				return err
			}
		}
		if err := d.s2SendCommand(0x61); err != nil {
			return err
		}
		for _, v := range []byte{0x02, 0x88, 0x01, 0xEC} {
			if err := d.s2SendData(v); err != nil {
				return err
			}
		}

		if err := d.m1s1m2s2SendCommand(0x15); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x20); err != nil {
			return err
		}

		if err := d.m1s1m2s2SendCommand(0x50); err != nil {
			return err
		}
		for _, v := range []byte{0x11, 0x07} {
			if err := d.m1s1m2s2SendData(v); err != nil {
				return err
			}
		}

		if err := d.m1s1m2s2SendCommand(0x60); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x22); err != nil {
			return err
		}

		if err := d.m1s1m2s2SendCommand(0xE3); err != nil {
			return err
		}
		if err := d.m1s1m2s2SendData(0x00); err != nil {
			return err
		}

		// Temperature read is optional; for now we can skip sending it back (0xe0/e5)
		// or implement a direct port later.
	default:
		return fmt.Errorf("epd: unsupported panel version %d", panelVersion)
	}

	return nil
}

// Clear is the Go equivalent of EPD_12in48B_Clear.
func (d *Driver) Clear() error {
	// M1 part 648*492
	if err := d.m1SendCommand(0x10); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 0; x < 81; x++ {
			if err := d.m1SendData(0xff); err != nil {
				return err
			}
		}
	}
	if err := d.m1SendCommand(0x13); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 0; x < 81; x++ {
			if err := d.m1SendData(0x00); err != nil {
				return err
			}
		}
	}

	// S1 part 656*492
	if err := d.s1SendCommand(0x10); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 81; x < 163; x++ {
			if err := d.s1SendData(0xff); err != nil {
				return err
			}
		}
	}
	if err := d.s1SendCommand(0x13); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 81; x < 163; x++ {
			if err := d.s1SendData(0x00); err != nil {
				return err
			}
		}
	}

	// M2 part 656*492
	if err := d.m2SendCommand(0x10); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 81; x < 163; x++ {
			if err := d.m2SendData(0xff); err != nil {
				return err
			}
		}
	}
	if err := d.m2SendCommand(0x13); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 81; x < 163; x++ {
			if err := d.m2SendData(0x00); err != nil {
				return err
			}
		}
	}

	// S2 part 648*492
	if err := d.s2SendCommand(0x10); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 0; x < 81; x++ {
			if err := d.s2SendData(0xff); err != nil {
				return err
			}
		}
	}
	if err := d.s2SendCommand(0x13); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 0; x < 81; x++ {
			if err := d.s2SendData(0x00); err != nil {
				return err
			}
		}
	}

	// Turn On Display
	return d.TurnOnDisplay()
}

// Display is the Go equivalent of EPD_12in48B_Display.
// BlackImage and RedImage must each be 163*984 bytes.
func (d *Driver) Display(black, red []byte) error {
	if len(black) != 163*984 || len(red) != 163*984 {
		return fmt.Errorf("epd: invalid buffer size, expected %d bytes per plane", 163*984)
	}

	// S2 part 648*492 (top-left)
	if err := d.s2SendCommand(0x10); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 0; x < 81; x++ {
			b := black[y*163+x]
			if err := d.s2SendData(b); err != nil {
				return err
			}
		}
	}
	if err := d.s2SendCommand(0x13); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 0; x < 81; x++ {
			r := ^red[y*163+x]
			if err := d.s2SendData(r); err != nil {
				return err
			}
		}
	}

	// M2 part 656*492 (top-right)
	if err := d.m2SendCommand(0x10); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 81; x < 163; x++ {
			b := black[y*163+x]
			if err := d.m2SendData(b); err != nil {
				return err
			}
		}
	}
	if err := d.m2SendCommand(0x13); err != nil {
		return err
	}
	for y := 0; y < 492; y++ {
		for x := 81; x < 163; x++ {
			r := ^red[y*163+x]
			if err := d.m2SendData(r); err != nil {
				return err
			}
		}
	}

	// S1 part 656*492 (bottom-right)
	if err := d.s1SendCommand(0x10); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 81; x < 163; x++ {
			b := black[y*163+x]
			if err := d.s1SendData(b); err != nil {
				return err
			}
		}
	}
	if err := d.s1SendCommand(0x13); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 81; x < 163; x++ {
			r := ^red[y*163+x]
			if err := d.s1SendData(r); err != nil {
				return err
			}
		}
	}

	// M1 part 648*492 (bottom-left)
	if err := d.m1SendCommand(0x10); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 0; x < 81; x++ {
			b := black[y*163+x]
			if err := d.m1SendData(b); err != nil {
				return err
			}
		}
	}
	if err := d.m1SendCommand(0x13); err != nil {
		return err
	}
	for y := 492; y < 984; y++ {
		for x := 0; x < 81; x++ {
			r := ^red[y*163+x]
			if err := d.m1SendData(r); err != nil {
				return err
			}
		}
	}

	return d.TurnOnDisplay()
}

// TurnOnDisplay is the Go equivalent of EPD_12in48B_TurnOnDisplay.
func (d *Driver) TurnOnDisplay() error {
	// power on
	if err := d.m1m2SendCommand(0x04); err != nil {
		return err
	}
	delayMs(300)

	// Display Refresh
	if err := d.m1s1m2s2SendCommand(0x12); err != nil {
		return err
	}

	// Wait for all segments to become idle.
	d.m1ReadBusy()
	d.s1ReadBusy()
	d.m2ReadBusy()
	d.s2ReadBusy()

	return nil
}

// Sleep is the Go equivalent of EPD_12in48B_Sleep.
func (d *Driver) Sleep() error {
	if err := d.m1s1m2s2SendCommand(0x02); err != nil { // power off
		return err
	}
	delayMs(300)

	if err := d.m1s1m2s2SendCommand(0x07); err != nil { // deep sleep
		return err
	}
	if err := d.m1s1m2s2SendData(0xA5); err != nil {
		return err
	}
	delayMs(300)
	return nil
}
