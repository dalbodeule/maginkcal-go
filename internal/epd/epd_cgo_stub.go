//go:build !(linux && arm && cgo)

// Skeleton CDriver implementation for non-linux/arm/cgo targets.
//
// 이 파일은 linux/arm + cgo 환경이 아닐 때 빌드된다. 실제 C 기반 EPD 드라이버는
// linux/arm + cgo 조합에서만 활성화되며, 그 외 플랫폼에서는 아래 스켈레톤이
// 링크되도록 해서 전체 패키지가 항상 빌드 가능하도록 한다.

package epd

import "fmt"

// CDriver 는 linux/arm + cgo 환경에서만 실제 구현을 제공하며,
// 그 외 플랫폼에서는 no-op / 에러를 반환하는 스켈레톤으로 동작한다.
type CDriver struct{}

// InitC 는 비 linux/arm/cgo 환경에서는 항상 에러를 반환한다.
func InitC() (*CDriver, error) {
	return nil, fmt.Errorf("epd(cgo): C driver is only available on linux/arm with cgo enabled")
}

// Clear 는 non-linux/arm/cgo 환경에서는 아무 것도 하지 않는다.
func (d *CDriver) Clear() {}

// Display 는 non-linux/arm/cgo 환경에서는 에러를 반환한다.
func (d *CDriver) Display(black, red []byte) error {
	return fmt.Errorf("epd(cgo): Display is not supported on this platform")
}

// Sleep 는 non-linux/arm/cgo 환경에서는 아무 것도 하지 않는다.
func (d *CDriver) Sleep() {}
