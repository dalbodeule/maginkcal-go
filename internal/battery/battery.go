package battery

import (
	"context"
	"errors"
	"math/rand"
	"runtime"
	"time"

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

// i2cReader talks to a real battery controller over I2C. The intended
// target is PiSugar3, which exposes:
//   - 0x22 (high), 0x23 (low): battery voltage in millivolts
//   - 0x2A: battery percentage (0–100)
type i2cReader struct {
	busName string
	addr    uint16
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

// NewI2CReader constructs an I2C-backed Reader.
//
//   - busName: I2C bus identifier for periph.io ("" for default, typically /dev/i2c-1 on Raspberry Pi)
//   - addr:    7-bit I2C address of the battery controller (PiSugar3는 일반적으로 0x75 사용)
//
// 이 함수는 단순히 구성을 보관만 하고, 실제 I2C 연결/host.Init은 Read 시점에 수행한다.
func NewI2CReader(busName string, addr uint16) Reader {
	if busName == "" {
		busName = ""
	}
	return &i2cReader{
		busName: busName,
		addr:    addr,
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

// Read implements Reader for the I2C-backed reader.
func (r *i2cReader) Read(_ context.Context) (Status, error) {
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
		w := []byte{reg}
		buf := []byte{0}
		if err := dev.Tx(w, buf); err != nil {
			return 0, err
		}
		return buf[0], nil
	}

	// Voltage (mV): high at 0x22, low at 0x23
	high, err := readReg(0x22)
	if err != nil {
		return Status{}, err
	}
	low, err := readReg(0x23)
	if err != nil {
		return Status{}, err
	}
	voltageMv := int(uint16(high)<<8 | uint16(low))

	// Percent: 0x2A
	pct, err := readReg(0x2A)
	if err != nil {
		return Status{}, err
	}
	if pct > 100 {
		pct = 100
	}

	return Status{
		Percent:   int(pct),
		VoltageMv: voltageMv,
	}, nil
}

// DefaultReader returns the Reader that should be used by the main program.
//
// 우선순위:
//  1. Linux/ARM 환경에서 I2C(PiSugar3 등) 사용을 시도
//  2. 실패 시 mock 리더로 fallback
//
// 이렇게 하면 Web UI 및 HTTP 핸들러는 Reader 인터페이스만 사용하고,
// 실제 하드웨어가 없거나 초기화 실패 시에도 안전하게 동작한다.
func DefaultReader() Reader {
	// linux/arm 이 아닌 경우에는 바로 mock 으로.
	if runtime.GOOS != "linux" {
		return NewMockReader()
	}

	// PiSugar3 기본값으로 알려진 I2C 주소(0x75)를 사용한다.
	const defaultAddr = 0x57

	// I2C 리더를 구성하고, 한 번 읽기 테스트를 해본 뒤 실패하면 mock 으로 fallback.
	r := NewI2CReader("", defaultAddr)
	if _, err := r.Read(context.Background()); err != nil {
		// I2C 접근 실패 시 로그는 battery 패키지 레벨에서 직접 찍지 않고,
		// 상위에서 Percent=0, VoltageMv=0 등을 보고 판단하게 할 수도 있다.
		// 여기서는 조용히 mock 으로 대체한다.
		return NewMockReader()
	}
	return r
}
