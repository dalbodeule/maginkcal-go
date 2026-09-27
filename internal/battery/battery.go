package battery

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

// Status represents current battery status for Web UI / API.
type Status struct {
	// Percent is the battery level in 0–100%.
	Percent int `json:"percent"`
	// VoltageMv is the battery voltage in millivolts, if known.
	VoltageMv int `json:"voltage_mv"`
}

// Reader abstracts how we obtain battery information.
type Reader interface {
	Read(ctx context.Context) (Status, error)
}

// i2cReader talks to a real battery controller over I2C. The intended
// target is PiSugar3, which exposes:
//   - 0x22 (high), 0x23 (low): battery voltage in millivolts
//   - 0x2A: battery percentage (0–100)
type i2cReader struct {
	busName string
	addr    uint16
}

// NewI2CReader constructs an I2C-backed Reader.
//
//   - busName: I2C bus identifier for periph.io ("1" for /dev/i2c-1)
//   - addr:    7-bit I2C address of the battery controller (PiSugar 3 defaults to 0x57)
//
// 이 함수는 단순히 구성을 보관만 하고, 실제 I2C 연결/host.Init은 Read 시점에 수행한다.
func NewI2CReader(busName string, addr uint16) Reader {
	return &i2cReader{
		busName: busName,
		addr:    addr,
	}
}

// Read implements Reader for the I2C-backed reader.
func (r *i2cReader) Read(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	// 플랫폼 체크: Linux/ARM 이 아닌 경우에는 I2C를 시도하지 않는다.
	if runtime.GOOS != "linux" {
		return Status{}, errors.New("battery: i2c reader unavailable on this platform")
	}
	// periph 초기화
	if _, err := host.Init(); err != nil {
		return Status{}, err
	}

	bus, err := i2creg.Open(r.busName)
	if err != nil {
		return Status{}, err
	}
	defer bus.Close()

	dev := &i2c.Dev{Bus: bus, Addr: r.addr}

	readReg := func(reg byte) (byte, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		w := []byte{reg}
		buf := []byte{0}
		if err := dev.Tx(w, buf); err != nil {
			return 0, err
		}
		return buf[0], nil
	}

	// Read the required percentage first; an invalid value must not look full.
	pct, err := readReg(0x2A)
	if err != nil {
		return Status{}, err
	}
	if pct > 100 {
		return Status{}, fmt.Errorf("battery: invalid percentage %d", pct)
	}

	// Voltage (mV): high at 0x22, low at 0x23.
	high, err := readReg(0x22)
	if err != nil {
		if ctx.Err() != nil {
			return Status{}, ctx.Err()
		}
		return Status{Percent: int(pct)}, nil
	}
	low, err := readReg(0x23)
	if err != nil {
		if ctx.Err() != nil {
			return Status{}, ctx.Err()
		}
		return Status{Percent: int(pct)}, nil
	}
	voltageMv := int(uint16(high)<<8 | uint16(low))

	return Status{
		Percent:   int(pct),
		VoltageMv: voltageMv,
	}, nil
}

// DefaultReader reads PiSugar 3 directly from the I2C bus allowed by systemd.
// Read failures are reported to the caller so unavailable hardware is not
// mistaken for a real battery percentage.
func DefaultReader() Reader {
	return NewI2CReader("1", 0x57)
}
