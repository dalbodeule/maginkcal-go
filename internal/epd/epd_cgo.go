//go:build linux && arm && cgo

// cgo-backed EPD driver wrapper.
//
// 이 파일은 linux/arm(cgo 활성화) 환경에서만 활성화된다. 즉,
//   - GOOS=linux
//   - GOARCH=arm (예: armv7)
//   - CGO_ENABLED=1
// 일 때만 실제 C 드라이버에 링크된다.
//
// 내용:
//   - Waveshare C 드라이버(DEV_Config.c + EPD_12in48B_* 등)를 Go 코드에서
//     직접 호출하기 위한 래퍼.
//   - Zero 2 W + trixie 환경에서 C 구현이 정상 동작하는 것이 확인되었으며,
//     여기서는 그 C API를 cgo로 감싸는 역할만 한다.
//
// 전제:
//   - internal/epd/c/ 디렉터리 아래에 C 헤더/라이브러리가 존재한다.
//   - 예: internal/epd/c/EPD_12in48B.h, DEV_Config.h, libepddrv.a 등.
//   - C 쪽에는 아래와 같은 심벌이 제공된다고 가정한다:
//
//       UBYTE DEV_ModuleInit(void);
//       void  DEV_ModuleExit(void);
//
//       void  EPD_12in48B_Init(void);
//       void  EPD_12in48B_Clear(void);
//       void  EPD_12in48B_Display(const unsigned char *black,
//                                 const unsigned char *red);
//       void  EPD_12in48B_Sleep(void);
//
//   - 실제 함수명/헤더명은 Waveshare 예제에 맞춰 수정하면 된다.
//
// 빌드 시에는 cgo가 internal/epd/c 디렉터리에서 헤더/라이브러리를
// 찾을 수 있도록 아래 #cgo CFLAGS/LDFLAGS를 적절히 조정해야 한다.

package epd

/*
#cgo linux,arm CFLAGS: -I${SRCDIR}/c
#cgo linux,arm LDFLAGS: -L${SRCDIR}/c -lepddrv -llgpio

#include <stdint.h>
#include "EPD_12in48b.h"
#include "DEV_Config.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// CDriver 는 Waveshare C 드라이버를 직접 호출하는 래퍼 타입이다.
// 순수 Go 드라이버(Dev/Driver)와는 별도이므로, 필요에 따라
// 내부에서 선택적으로 사용하면 된다.
type CDriver struct{}

// InitC 는 C 쪽 DEV_ModuleInit + EPD_12in48B_Init 을 호출해
// 패널을 초기화한다.
//
// 일반적인 사용 예:
//
//	d, err := epd.InitC()
//	if err != nil { ... }
//	defer d.Sleep()
func InitC() (*CDriver, error) {
	// DEV_ModuleInit() 이 0 이면 성공, 비 0 이면 실패라고 가정.
	if ret := C.DEV_ModuleInit(); ret != 0 {
		return nil, fmt.Errorf("epd(cgo): DEV_ModuleInit failed (ret=%d)", int(ret))
	}

	// 패널 초기화
	C.EPD_12in48B_Init()

	return &CDriver{}, nil
}

// Clear 는 C 드라이버의 EPD_12in48B_Clear()를 그대로 호출한다.
func (d *CDriver) Clear() {
	C.EPD_12in48B_Clear()
}

// Display 는 C 드라이버의 EPD_12in48B_Display()를 호출한다.
// black, red 버퍼는 Go 순수 드라이버와 동일하게 163*984 바이트(1304x984/8)를
// 기대한다.
func (d *CDriver) Display(black, red []byte) error {
	const planeSize = 163 * 984

	if len(black) != planeSize || len(red) != planeSize {
		return fmt.Errorf("epd(cgo): invalid buffer size, expected %d bytes per plane", planeSize)
	}
	if len(black) == 0 || len(red) == 0 {
		return fmt.Errorf("epd(cgo): empty buffers")
	}

	cb := (*C.uchar)(unsafe.Pointer(&black[0]))
	cr := (*C.uchar)(unsafe.Pointer(&red[0]))

	C.EPD_12in48B_Display(cb, cr)
	return nil
}

// Sleep 은 C 드라이버의 EPD_12in48B_Sleep() + DEV_ModuleExit()를 호출한다.
func (d *CDriver) Sleep() {
	C.EPD_12in48B_Sleep()
	C.DEV_ModuleExit()
}
