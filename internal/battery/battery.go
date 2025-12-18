package battery

import (
	"context"
	"math/rand"
	"time"
)

// Status represents current battery status for Web UI / API.
type Status struct {
	// Percent is the battery level in 0–100%.
	Percent int `json:"percent"`
	// VoltageMv is the battery voltage in millivolts, if known.
	VoltageMv int `json:"voltage_mv"`
}

// Reader abstracts how we obtain battery information. This allows us to have
// a mock implementation for development and an actual PiSugar3 I2C-backed
// implementation for Raspberry Pi.
type Reader interface {
	Read(ctx context.Context) (Status, error)
}

// mockReader is used for demo/development. It returns a pseudo-random
// percentage and no real voltage information.
type mockReader struct {
	rnd *rand.Rand
}

// NewMockReader constructs a mock Reader that generates random percentages.
// This is suitable for:
//   - local development on non-Raspberry Pi machines
//   - demo mode before wiring real I2C access
func NewMockReader() Reader {
	return &mockReader{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *mockReader) Read(_ context.Context) (Status, error) {
	// Demo behaviour:
	// - Random percentage between 20% and 100%.
	// - Voltage is left as 0 (unknown) for now.
	p := 20 + m.rnd.Intn(81) // 20..100 inclusive
	return Status{
		Percent:   p,
		VoltageMv: 0,
	}, nil
}

// DefaultReader returns the Reader that should be used by the main program.
//
// For now this always returns the mock implementation. Later, this function
// can be changed (or split by build tags) to return a PiSugar3-backed reader
// on Raspberry Pi:
//
//   - Use /dev/i2c-* (e.g. /dev/i2c-1) with the PiSugar3 I2C address.
//   - Read registers:
//     0x22 (high byte), 0x23 (low byte)  => voltage in mV
//     0x2A                               => battery percentage (0–100)
//   - Combine 0x22 and 0x23 like: (high << 8) | low = millivolts
//
// This keeps the Web UI and HTTP handlers stable while allowing the
// underlying implementation to switch from mock to real hardware.
func DefaultReader() Reader {
	return NewMockReader()
}
