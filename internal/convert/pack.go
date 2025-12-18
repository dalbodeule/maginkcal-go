package convert

import (
	"fmt"
	"image"
	"image/color"
)

// EPD panel geometry (12.48" B, tri-color).
const (
	EPDWidth      = 1304
	EPDHeight     = 984
	EPDByteStride = EPDWidth / 8 // 163 bytes per row
	EPDPlaneSize  = EPDByteStride * EPDHeight
)

// PackNRGBA converts an image.NRGBA into packed 1bpp black/red planes suitable
// for the Waveshare 12.48" B panel.
//
// Requirements / behavior:
//
//   - img width must be exactly 1304 pixels (EPDWidth).
//   - img height must be >= 984 pixels (EPDHeight).
//   - height가 더 크면 세로 방향으로 중앙을 잘라(센터 크롭) 984px만 사용한다.
//   - 픽셀 분류:
//   - 투명(alpha < 128) → white
//   - 매우 어두운 픽셀 → black plane에 잉크
//   - 충분히 "빨간" 픽셀 → red plane에 잉크
//   - 나머지 → white
//
// Packing 규칙:
//
//   - 각 plane은 y-major, MSB-first 1bpp:
//     byteIndex = y * 163 + (x >> 3)
//     mask      = 0x80 >> (x & 7)
//   - 초기값은 모든 비트를 1(white)로 채우고,
//     잉크가 필요한 픽셀만 해당 비트를 0으로 클리어한다.
func PackNRGBA(img *image.NRGBA) (black, red []byte, err error) {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	if w != EPDWidth {
		return nil, nil, fmt.Errorf("convert: expected width %d, got %d", EPDWidth, w)
	}
	if h < EPDHeight {
		return nil, nil, fmt.Errorf("convert: expected height >= %d, got %d", EPDHeight, h)
	}

	// 세로 방향으로 가운데 984px만 사용 (센터 크롭).
	startY := b.Min.Y + (h-EPDHeight)/2

	black = make([]byte, EPDPlaneSize)
	red = make([]byte, EPDPlaneSize)

	// 초기값은 모두 white(1)로 설정.
	for i := range black {
		black[i] = 0xFF
	}
	for i := range red {
		red[i] = 0xFF
	}

	// 메인 루프: 이미지 stride를 직접 사용해 At() 호출을 피한다.
	for py := 0; py < EPDHeight; py++ {
		srcY := startY + py
		rowOff := (srcY - b.Min.Y) * img.Stride

		for px := 0; px < EPDWidth; px++ {
			// srcX는 bounds.Min.X 기준으로 계산한다.
			srcX := b.Min.X + px
			colOff := (srcX - b.Min.X) * 4
			i := rowOff + colOff

			r := img.Pix[i+0]
			g := img.Pix[i+1]
			bb := img.Pix[i+2]
			a := img.Pix[i+3]

			// 완전 투명/반투명은 화면에서 보이지 않는다고 가정하고 white 취급.
			if a < 128 {
				continue
			}

			ink := classifyPixel(color.NRGBA{R: r, G: g, B: bb, A: a})

			if ink == inkWhite {
				continue
			}

			byteIndex := py*EPDByteStride + (px >> 3)
			mask := byte(0x80 >> (px & 7))

			switch ink {
			case inkBlack:
				black[byteIndex] &^= mask // 0=black ink
			case inkRed:
				red[byteIndex] &^= mask // 0=red ink (C 드라이버에서 ~ 처리)
			}
		}
	}

	return black, red, nil
}

// inkColor indicates which plane a pixel should be drawn to.
type inkColor int

const (
	inkWhite inkColor = iota
	inkBlack
	inkRed
)

// classifyPixel decides whether a pixel should be black, red, or white on the
// tri-color panel.
//
// 기준(경험적):
//
//   - 밝기 Y = 0.299R + 0.587G + 0.114B
//
//   - redness = R - max(G, B)
//
//   - 매우 어두운 픽셀(Y < 64) → black
//
//   - 충분히 밝고(redness > 32, R > 128) → red
//
//   - 나머지 → white
func classifyPixel(c color.NRGBA) inkColor {
	r, g, b := float64(c.R), float64(c.G), float64(c.B)

	// Luma (perceptual brightness).
	y := 0.299*r + 0.587*g + 0.114*b

	// Red dominance.
	maxGB := g
	if b > maxGB {
		maxGB = b
	}
	redness := r - maxGB

	// 어둡고 채도가 높지 않은 픽셀은 black으로.
	if y < 64 {
		return inkBlack
	}

	// 충분히 빨간 계열은 red로.
	if r > 128 && redness > 32 {
		return inkRed
	}

	return inkWhite
}
