package convert

import (
	"fmt"
	"image"
	"image/color"
)

// EPD panel geometry (12.48" B, tri-color).
//
// Waveshare 문서 기준 물리 해상도는 가로 1304, 세로 984 이고,
// C 드라이버는 width=1304, height=984, stride=163(=1304/8)을 사용한다.
const (
	EPDWidth      = 1304
	EPDHeight     = 984
	EPDByteStride = EPDWidth / 8 // 163 bytes per row
	EPDPlaneSize  = EPDByteStride * EPDHeight

	// Web UI / 캡처용 logical 이미지 해상도(세로 레이아웃).
	// 캡처 PNG 는 984 x 1304 정도의 세로형 이미지이므로,
	// 이를 90 또는 270도 회전시켜 패널에 맞춘다.
	srcWidth     = 984  // 캡처된 이미지의 가로
	srcMinHeight = 1304 // 캡처된 이미지의 최소 세로(이 이상이면 센터 크롭)
)

// PackNRGBA converts an image.NRGBA into packed 1bpp black/red planes suitable
// for the Waveshare 12.48" B panel.
//
// rotation 인자:
//
//   - 90  : 세로 이미지를 **시계 방향 90도** 회전해서 패널(1304x984)에 매핑
//   - 270 : 세로 이미지를 **반시계 방향 90도(=시계 270도)** 회전해서 매핑
//   - 그 외 값은 90도로 강제한다.
//
// Requirements / behavior:
//
//   - img width must be exactly 984 pixels (srcWidth).
//   - img height must be >= 1304 pixels (srcMinHeight).
//   - height가 더 크면 세로 방향으로 중앙을 잘라(센터 크롭) 1304px만 사용한다.
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
func PackNRGBA(img *image.NRGBA, rotation int) (black, red []byte, err error) {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	if w != srcWidth {
		return nil, nil, fmt.Errorf("convert: expected width %d, got %d", srcWidth, w)
	}
	if h < srcMinHeight {
		return nil, nil, fmt.Errorf("convert: expected height >= %d, got %d", srcMinHeight, h)
	}

	// rotation 정규화: 90 / 270 만 허용, 나머지는 90으로 처리.
	if rotation != 90 && rotation != 270 {
		rotation = 90
	}

	// 세로 방향으로 가운데 srcMinHeight px만 사용 (센터 크롭).
	startY := b.Min.Y + (h-srcMinHeight)/2

	black = make([]byte, EPDPlaneSize)
	red = make([]byte, EPDPlaneSize)

	// 초기값은 모두 white(1)로 설정.
	for i := range black {
		black[i] = 0xFF
	}
	for i := range red {
		red[i] = 0xFF
	}

	// 메인 루프:
	//
	// 패널 좌표계 (destX, destY):
	//   - 0 <= destX < 1304, 0 <= destY < 984
	//
	// 소스(캡처) 좌표계 (srcX, srcY):
	//   - 0 <= srcX < 984
	//   - crop 내에서 0 <= (srcY - startY) < 1304
	//
	// 회전 변환:
	//
	//   1) 시계 방향 90도 (rotation == 90)
	//      destX = srcH - 1 - srcY
	//      destY = srcX
	//      역변환:
	//        srcX = destY
	//        srcY = srcH - 1 - destX
	//
	//   2) 반시계 방향 90도 (rotation == 270)
	//      destX = srcY
	//      destY = srcW - 1 - srcX
	//      역변환:
	//        srcX = srcW - 1 - destY
	//        srcY = destX
	//
	//   여기서 srcH = srcMinHeight, srcW = srcWidth 이며,
	//   srcY 에는 crop offset(startY)을 더해준다.
	for destY := 0; destY < EPDHeight; destY++ {
		for destX := 0; destX < EPDWidth; destX++ {
			var srcX, srcY int

			if rotation == 90 {
				// 시계 방향 90도
				srcX = b.Min.X + destY
				srcY = startY + (srcMinHeight - 1 - destX)
			} else {
				// rotation == 270 : 반시계 방향 90도
				srcX = b.Min.X + (srcWidth - 1 - destY)
				srcY = startY + destX
			}

			rowOff := (srcY - b.Min.Y) * img.Stride
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

			byteIndex := destY*EPDByteStride + (destX >> 3)
			mask := byte(0x80 >> (destX & 7))

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
//   - redness = R - max(G, B)
//   - 매우 어두운 픽셀(Y < 64) → black
//   - 충분히 밝고(redness > 32, R > 128) → red
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
