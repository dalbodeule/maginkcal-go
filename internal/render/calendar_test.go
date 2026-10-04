package render

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"epdcal/internal/convert"
	"github.com/go-text/typesetting/fontscan"
)

func TestLoadFacesFailsWhenEnvironmentAndSystemFontsCannotResolve(t *testing.T) {
	_, err := loadFacesWithPaths("/missing/korean-font.ttf", "", "", func() ([]fontscan.Footprint, error) {
		return nil, nil
	})
	if !errors.Is(err, ErrFontUnavailable) {
		t.Fatalf("font error = %v, want ErrFontUnavailable", err)
	}
}

func TestLoadFacesUsesExplicitKoreanFontWithoutSystemScan(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("Nanum Gothic or Apple Korean system font is not installed")
	}
	faces, err := loadFacesWithPaths(fontPath, "", "", func() ([]fontscan.Footprint, error) {
		t.Fatal("explicit usable font should avoid scanning system fonts")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	faces.close()
}

func TestLoadFacesCanUseExplicitBoldFontAsFallback(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("Nanum Gothic or Apple Korean system font is not installed")
	}
	faces, err := loadFacesWithPaths("", fontPath, "", func() ([]fontscan.Footprint, error) {
		t.Fatal("usable explicit bold font should avoid scanning system fonts")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if faces.regular != faces.bold {
		t.Fatal("bold-only override should be used as the regular face fallback")
	}
	faces.close()
}

func TestLoadFacesFallsBackToScannedSystemFonts(t *testing.T) {
	faces, err := loadFacesWithPaths("/missing/korean-font.ttf", "", "", func() ([]fontscan.Footprint, error) {
		return fontscan.SystemFonts(nil, t.TempDir())
	})
	if errors.Is(err, ErrFontUnavailable) {
		t.Skip("host has no system font with the required Korean glyphs")
	}
	if err != nil {
		t.Fatal(err)
	}
	faces.close()
}

func TestNormalizeFontFamilyAliases(t *testing.T) {
	for input, want := range map[string]string{
		"나눔고딕":         "nanumgothic",
		"Nanum Gothic": "nanumgothic",
		"Noto Sans KR": "notosanscjkkr",
	} {
		if got := normalizeFontFamily(input); got != want {
			t.Errorf("normalizeFontFamily(%q) = %q, want %q", input, got, want)
		}
	}
	if !fontFamilyMatches("Noto Sans CJK KR", normalizeFontFamily("Noto Sans KR")) {
		t.Fatal("Noto Sans KR should match the Noto Sans CJK KR system family")
	}
}

func TestLoadFacesSelectsRequestedScannedFamily(t *testing.T) {
	candidates, err := fontscan.SystemFonts(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	family := ""
	for _, candidate := range candidates {
		if candidate.Aspect.Weight < 600 && hasRequiredGlyphs(candidate) {
			if _, err := readFontAt(candidate.Location.File, candidate.Location.Index); err == nil {
				family = candidate.Family
				break
			}
		}
	}
	if family == "" {
		t.Skip("host has no parseable system font with the required Korean glyphs")
	}

	faces, err := loadFacesWithPaths("", "", family, func() ([]fontscan.Footprint, error) {
		return candidates, nil
	})
	if err != nil {
		t.Fatalf("requested family %q was not selected: %v", family, err)
	}
	faces.close()
}

func TestRenderCalendarCreatesPanelSizedImage(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("Nanum Gothic or Apple Korean system font is not installed")
	}
	t.Setenv("EPDCAL_FONT_REGULAR", fontPath)
	t.Setenv("EPDCAL_FONT_BOLD", fontPath)

	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	battery := 67
	temp := 18.6
	img, err := RenderCalendar(Data{
		Now:       time.Date(2026, 10, 4, 17, 0, 0, 0, loc),
		Locale:    "ko",
		Timezone:  "Asia/Seoul",
		WeekStart: "monday",
		Occurrences: []Occurrence{{
			Summary:      "내부 렌더러 일정 확인",
			Start:        time.Date(2026, 10, 5, 9, 0, 0, 0, loc),
			End:          time.Date(2026, 10, 5, 10, 0, 0, 0, loc),
			HighlightRed: true,
		}},
		HolidayDates: []string{"2026-10-04"},
		Battery:      &battery,
		Weather:      &Weather{Location: "Seoul", Current: "구름 조금", CurrentTemp: &temp},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got.X != Width || got.Y != Height {
		t.Fatalf("render size = %v, want %dx%d", got, Width, Height)
	}
	for _, p := range []image.Point{{0, 0}, {Width - 1, 0}, {0, Height - 1}, {Width - 1, Height - 1}} {
		if got := img.NRGBAAt(p.X, p.Y); got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
			t.Errorf("corner pixel %v = %+v, want plain white", p, got)
		}
	}

	// October 4, 2026 is Sunday in the first calendar row. Keep its red
	// appearance in the packed red plane as well as in the PNG preview.
	blackPlane, redPlane, err := convert.PackNRGBA(img, 90)
	if err != nil {
		t.Fatal(err)
	}
	// The lightly tinted empty area of today's cell must remain white on the
	// panel instead of turning the entire highlighted cell black.
	const todayBackgroundX, todayBackgroundY = 900, 300
	destX, destY := convert.EPDWidth-1-todayBackgroundY, todayBackgroundX
	backgroundIndex := destY*convert.EPDByteStride + destX/8
	backgroundMask := byte(0x80 >> (destX % 8))
	if blackPlane[backgroundIndex]&backgroundMask == 0 || redPlane[backgroundIndex]&backgroundMask == 0 {
		t.Fatal("today cell background was packed as ink instead of white")
	}

	redInDateCell := false
	for y := 180; y < 230; y++ {
		for x := 820; x < 960; x++ {
			c := img.NRGBAAt(x, y)
			if c.R > 150 && c.G < 100 && c.B < 100 {
				destX, destY := 1303-y, x
				index := destY*convert.EPDByteStride + destX/8
				mask := byte(0x80 >> (destX % 8))
				if redPlane[index]&mask == 0 && blackPlane[index]&mask != 0 {
					redInDateCell = true
				}
			}
		}
	}
	if !redInDateCell {
		t.Fatal("October 4 date pixels were not routed to the EPD red plane")
	}

	var dark, red int
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.R < 100 && c.G < 100 && c.B < 100 {
				dark++
			}
			if c.R > 150 && c.G < 100 && c.B < 100 {
				red++
			}
		}
	}
	if dark < 100 || red < 20 {
		t.Fatalf("render ink counts too low: dark=%d red=%d", dark, red)
	}

	if output := os.Getenv("EPDCAL_RENDER_TEST_OUTPUT"); output != "" {
		f, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDrawBatteryOmitsUnknownReading(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < 40; y++ {
		for x := Width - 100; x < Width; x++ {
			img.SetNRGBA(x, y, white)
		}
	}
	drawBattery(img, nil, nil)
	for y := 0; y < 40; y++ {
		for x := Width - 100; x < Width; x++ {
			if got := img.NRGBAAt(x, y); got != white {
				t.Fatalf("unknown battery reading drew pixels at (%d,%d): %+v", x, y, got)
			}
		}
	}
}

func findTestFont() string {
	for _, path := range []string{
		"/usr/share/fonts/truetype/nanum/NanumGothic.ttf",
		"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
