package render

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"epdcal/internal/config"
	"github.com/go-text/typesetting/fontscan"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	Width  = 984
	Height = 1304
)

var ErrFontUnavailable = errors.New("render: no usable Korean font found")

type Occurrence struct {
	Summary      string    `json:"summary"`
	AllDay       bool      `json:"all_day"`
	HighlightRed bool      `json:"highlight_red"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
}

type Data struct {
	Now          time.Time
	Locale       string
	Timezone     string
	WeekStart    string
	Occurrences  []Occurrence `json:"occurrences"`
	HolidayDates []string     `json:"holiday_dates"`
	Battery      *int
	Layout       config.CalendarLayout
	AssetDir     string
	FontFamily   string
	Weather      *Weather
}

type Weather struct {
	Location    string
	Current     string
	CurrentTemp *float64
}

type faceSet struct {
	regular *opentype.Font
	bold    *opentype.Font
	faces   map[faceKey]font.Face
}

type faceKey struct {
	size int
	bold bool
}

func RenderCalendar(data Data) (*image.NRGBA, error) {
	if data.Layout.MaxEventsPerDay == 0 {
		data.Layout = config.DefaultCalendarLayout()
	}
	if data.Timezone == "" {
		data.Timezone = "Asia/Seoul"
	}
	loc, err := time.LoadLocation(data.Timezone)
	if err != nil {
		return nil, fmt.Errorf("render: invalid timezone %q: %w", data.Timezone, err)
	}
	if data.Now.IsZero() {
		data.Now = time.Now()
	}
	now := data.Now.In(loc)

	fonts, err := loadFaces(data.FontFamily)
	if err != nil {
		return nil, err
	}
	defer fonts.close()

	img := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	// Match the exported page's 24px inset, compact header and five-week grid.
	const inset = 24
	if data.Layout.ShowBattery {
		drawBattery(img, fonts, data.Battery)
	}

	title := formatDate(now, data.Locale)
	weekday := formatWeekday(now, data.Locale)
	meta := weekday + " · " + data.Timezone + " · " + now.Format("15:04")
	lastUpdated := localized("마지막 업데이트:", "Last updated:", data.Locale) + " " + formatDateTime(now, data.Locale)

	drawText(img, fonts, title, inset, 60, data.Layout.HeaderFontSize, color.NRGBA{R: 15, G: 23, B: 42, A: 255}, true)
	drawText(img, fonts, meta, inset, 85, 16, color.NRGBA{R: 51, G: 65, B: 85, A: 255}, true)
	drawText(img, fonts, lastUpdated, inset, 110, 14, color.NRGBA{R: 71, G: 85, B: 105, A: 255}, false)
	fillRect(img, inset, 126, Width-inset, 127, color.NRGBA{R: 226, G: 232, B: 240, A: 255})

	gridX := inset
	gridW := Width - inset*2
	weekdayY := 143
	weekdayH := 28
	gridY := data.Layout.GridTop
	weekdayY = gridY - weekdayH - 8
	gridH := Height - 22 - gridY

	weekStart := strings.ToLower(data.WeekStart)
	if weekStart != "sunday" {
		weekStart = "monday"
	}
	labels := weekdayLabels(data.Locale, weekStart)
	for col, label := range labels {
		labelColor := color.NRGBA{R: 71, G: 85, B: 105, A: 255}
		if (weekStart == "monday" && col >= 5) || (weekStart == "sunday" && (col == 0 || col == 6)) {
			labelColor = color.NRGBA{R: 220, G: 38, B: 38, A: 255}
		}
		left := gridX + col*gridW/7
		right := gridX + (col+1)*gridW/7
		drawCentered(img, fonts, label, left, weekdayY+20, right-left, 14, labelColor, true)
	}

	start := startOfWeek(now, weekStart)
	holidaySet := make(map[string]bool, len(data.HolidayDates))
	for _, key := range data.HolidayDates {
		holidaySet[key] = true
	}
	eventsByDate := make(map[string][]Occurrence)
	for _, event := range data.Occurrences {
		key := event.Start.In(loc).Format("2006-01-02")
		eventsByDate[key] = append(eventsByDate[key], event)
	}

	cellWidth := gridW - 2 - 6
	cellHeight := gridH - 2 - 4
	gridColor := color.NRGBA{R: 203, G: 213, B: 225, A: 255}
	fillRect(img, gridX, gridY, gridX+gridW, gridY+gridH, gridColor)
	for row := 0; row < 5; row++ {
		for col := 0; col < 7; col++ {
			x := gridX + 1 + col*cellWidth/7 + col
			y := gridY + 1 + row*cellHeight/5 + row
			right := gridX + 1 + (col+1)*cellWidth/7 + col
			bottom := gridY + 1 + (row+1)*cellHeight/5 + row

			day := start.AddDate(0, 0, row*7+col)
			key := day.Format("2006-01-02")
			currentMonth := day.Month() == now.Month() && day.Year() == now.Year()
			var bg color.Color = color.White
			if !currentMonth {
				bg = color.NRGBA{R: 248, G: 250, B: 252, A: 255}
			}
			today := sameDay(day, now)
			if today {
				bg = color.NRGBA{R: 241, G: 243, B: 246, A: 255}
			}
			fillRect(img, x, y, right, bottom, bg)

			weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
			dateColor := color.NRGBA{R: 15, G: 23, B: 42, A: 255}
			if !currentMonth {
				dateColor = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
			}
			if weekend || holidaySet[key] {
				dateColor = color.NRGBA{R: 220, G: 38, B: 38, A: 255}
			}

			dateX, dateY := x+7, y+26
			dateText := fmt.Sprint(day.Day())
			drawText(img, fonts, dateText, dateX, dateY, data.Layout.DateFontSize, dateColor, true)
			if today {
				dateWidth := measure(fonts, dateText, data.Layout.DateFontSize, true)
				fillRect(img, dateX, dateY+2, dateX+dateWidth, dateY+4, dateColor)
				drawText(img, fonts, localized("오늘", "Today", data.Locale), dateX+dateWidth+7, dateY+1, 20, color.NRGBA{R: 51, G: 65, B: 85, A: 255}, true)
			}

			events := eventsByDate[key]
			lineY := y + 48
			if len(events) == 0 && data.Layout.ShowEmptyDays {
				drawClipped(img, fonts, localized("일정 없음", "No events", data.Locale), x+7, lineY, right-x-14, 12, color.NRGBA{R: 148, G: 163, B: 184, A: 255}, true)
				continue
			}
			for i, event := range events {
				if i == data.Layout.MaxEventsPerDay {
					break
				}
				line := eventLine(event, loc, data.Locale)
				if !data.Layout.ShowEventTimes && !event.AllDay {
					line = event.Summary
				}
				ink := color.NRGBA{R: 15, G: 23, B: 42, A: 255}
				if event.HighlightRed {
					ink = color.NRGBA{R: 220, G: 38, B: 38, A: 255}
				}
				drawClipped(img, fonts, line, x+7, lineY+i*18, right-x-14, data.Layout.EventFontSize, ink, true)
			}
		}
	}
	if data.Weather != nil && data.Layout.ShowWeather {
		drawWeather(img, fonts, *data.Weather)
	}
	if data.AssetDir != "" {
		drawAssets(img, data.AssetDir, data.Layout.Assets)
	}
	return img, nil
}

func drawWeather(img *image.NRGBA, fonts *faceSet, weather Weather) {
	parts := make([]string, 0, 3)
	if location := strings.TrimSpace(weather.Location); location != "" {
		parts = append(parts, location)
	}
	if current := strings.TrimSpace(weather.Current); current != "" {
		parts = append(parts, current)
	}
	if weather.CurrentTemp != nil {
		parts = append(parts, fmt.Sprintf("%.0f°", *weather.CurrentTemp))
	}
	if len(parts) == 0 {
		return
	}
	ink := color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	label := strings.Join(parts, " · ")
	const left = Width / 2
	const right = Width - 24
	width := measure(fonts, label, 14, true)
	drawClipped(img, fonts, label, max(left, right-width), 110, right-left, 14, ink, true)
}

func drawAssets(dst *image.NRGBA, dir string, assets []config.AssetOverlay) {
	for _, asset := range assets {
		path := filepath.Join(dir, filepath.Base(asset.File))
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err != nil || info.Size() > 4<<20 {
			file.Close()
			continue
		}
		var src image.Image
		var cfg image.Config
		if strings.EqualFold(filepath.Ext(path), ".jpg") || strings.EqualFold(filepath.Ext(path), ".jpeg") {
			cfg, err = jpeg.DecodeConfig(file)
			if err == nil && (cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16_000_000) {
				err = fmt.Errorf("asset image dimensions exceed limit")
			}
			if err == nil {
				_, err = file.Seek(0, 0)
			}
			if err == nil {
				src, err = jpeg.Decode(file)
			}
		} else {
			cfg, err = png.DecodeConfig(file)
			if err == nil && (cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16_000_000) {
				err = fmt.Errorf("asset image dimensions exceed limit")
			}
			if err == nil {
				_, err = file.Seek(0, 0)
			}
			if err == nil {
				src, err = png.Decode(file)
			}
		}
		file.Close()
		if err != nil {
			continue
		}
		bounds := image.Rect(asset.X, asset.Y, asset.X+asset.Width, asset.Y+asset.Height)
		xdraw.CatmullRom.Scale(dst, bounds, src, src.Bounds(), draw.Over, nil)
	}
}

func loadFaces(family string) (*faceSet, error) {
	return loadFacesWithPaths(os.Getenv("EPDCAL_FONT_REGULAR"), os.Getenv("EPDCAL_FONT_BOLD"), family, func() ([]fontscan.Footprint, error) {
		return fontscan.SystemFonts(nil, "")
	})
}

func loadFacesWithPaths(regularPath, boldPath, family string, scan func() ([]fontscan.Footprint, error)) (*faceSet, error) {
	var regular, bold *opentype.Font
	var envFontErr error
	regularPath = strings.TrimSpace(regularPath)
	boldPath = strings.TrimSpace(boldPath)
	if regularPath != "" {
		regular, envFontErr = readFont(regularPath)
		if envFontErr == nil && !supportsRenderGlyphs(regular) {
			envFontErr = errors.New("font does not contain required Korean/display glyphs")
			regular = nil
		}
	}
	if boldPath != "" {
		var err error
		bold, err = readFont(boldPath)
		if err != nil || !supportsRenderGlyphs(bold) {
			bold = nil
		}
	}
	if regular == nil && bold != nil {
		regular = bold
	}

	var candidates []fontscan.Footprint
	if regular == nil {
		var err error
		candidates, err = scan()
		if err != nil {
			return nil, fmt.Errorf("%w: system font scan failed: %v; install a Korean font or set EPDCAL_FONT_REGULAR", ErrFontUnavailable, err)
		}
		var regularFamily string
		regular, regularFamily, bold = chooseSystemFonts(candidates, family)
		if regular == nil {
			cause := "system scan found no font with the required Korean glyphs"
			if strings.TrimSpace(family) != "" {
				cause = fmt.Sprintf("system scan found no usable font family matching %q", family)
			}
			if envFontErr != nil {
				cause = fmt.Sprintf("EPDCAL_FONT_REGULAR could not be used (%v); %s", envFontErr, cause)
			}
			return nil, fmt.Errorf("%w: %s; install fonts-nanum or set EPDCAL_FONT_REGULAR to a Korean-capable TTF/OTF", ErrFontUnavailable, cause)
		}
		if bold == nil {
			bold = chooseBoldFont(candidates, regularFamily)
		}
	}
	if bold == nil {
		fallbackBoldPath := filepath.Join(filepath.Dir(regularPath), "NanumGothicBold.ttf")
		if regularPath != "" && filepath.Dir(fallbackBoldPath) != "." {
			bold, _ = readFont(fallbackBoldPath)
		}
	}
	if bold == nil || !supportsRenderGlyphs(bold) {
		bold = regular
	}
	return &faceSet{regular: regular, bold: bold, faces: make(map[faceKey]font.Face)}, nil
}

func readFont(path string) (*opentype.Font, error) {
	return readFontAt(path, 0)
}

func readFontAt(path string, index uint16) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("ttcf")) {
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			return nil, err
		}
		if int(index) >= collection.NumFonts() {
			return nil, fmt.Errorf("font collection face index %d is out of range", index)
		}
		return collection.Font(int(index))
	}
	return opentype.Parse(data)
}

func CheckFonts(family string) error {
	faces, err := loadFaces(family)
	if err != nil {
		return err
	}
	faces.close()
	return nil
}

const requiredRenderGlyphs = "한글달력일정요일월화수목금토일오늘마지막업데이트없음0123456789/:·↑↓%+-°"

func supportsRenderGlyphs(file *opentype.Font) bool {
	face, err := opentype.NewFace(file, &opentype.FaceOptions{Size: 12, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return false
	}
	defer face.Close()
	for _, r := range requiredRenderGlyphs {
		if _, _, ok := face.GlyphBounds(r); !ok {
			return false
		}
	}
	return true
}

func chooseSystemFonts(fonts []fontscan.Footprint, requestedFamily string) (regular *opentype.Font, family string, bold *opentype.Font) {
	ordered := append([]fontscan.Footprint(nil), fonts...)
	requestedFamily = normalizeFontFamily(requestedFamily)
	if requestedFamily != "" {
		filtered := ordered[:0]
		for _, candidate := range ordered {
			if fontFamilyMatches(candidate.Family, requestedFamily) {
				filtered = append(filtered, candidate)
			}
		}
		ordered = filtered
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return fontPreference(ordered[i], false) < fontPreference(ordered[j], false)
	})
	for _, candidate := range ordered {
		if candidate.Aspect.Weight >= 600 || !hasRequiredGlyphs(candidate) {
			continue
		}
		parsed, err := readFontAt(candidate.Location.File, candidate.Location.Index)
		if err != nil || !supportsRenderGlyphs(parsed) {
			continue
		}
		regular, family = parsed, candidate.Family
		break
	}
	if regular == nil {
		return nil, "", nil
	}
	bold = chooseBoldFont(ordered, family)
	return regular, family, bold
}

func normalizeFontFamily(family string) string {
	family = strings.ToLower(strings.TrimSpace(family))
	family = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(family)
	switch family {
	case "나눔고딕", "nanumgothic":
		return "nanumgothic"
	case "notosanskr", "notosanscjkkr":
		return "notosanscjkkr"
	default:
		return family
	}
}

func fontFamilyMatches(candidate, requested string) bool {
	candidate = normalizeFontFamily(candidate)
	return candidate == requested || strings.Contains(candidate, requested)
}

func chooseBoldFont(fonts []fontscan.Footprint, family string) *opentype.Font {
	ordered := append([]fontscan.Footprint(nil), fonts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return fontPreference(ordered[i], true, family) < fontPreference(ordered[j], true, family)
	})
	for _, candidate := range ordered {
		if candidate.Aspect.Weight < 600 || !hasRequiredGlyphs(candidate) {
			continue
		}
		parsed, err := readFontAt(candidate.Location.File, candidate.Location.Index)
		if err == nil && supportsRenderGlyphs(parsed) {
			return parsed
		}
	}
	return nil
}

func hasRequiredGlyphs(candidate fontscan.Footprint) bool {
	for _, r := range requiredRenderGlyphs {
		if !candidate.Runes.Contains(r) {
			return false
		}
	}
	return true
}

func fontPreference(candidate fontscan.Footprint, wantBold bool, preferredFamily ...string) int {
	family := strings.ToLower(candidate.Family)
	familyRank := 20
	switch {
	case strings.Contains(family, "nanum"):
		familyRank = 0
	case strings.Contains(family, "noto") && strings.Contains(family, "cjk"):
		familyRank = 1
	case strings.Contains(family, "apple sd gothic"):
		familyRank = 2
	case strings.Contains(family, "malgun"):
		familyRank = 3
	case strings.Contains(family, "noto"):
		familyRank = 4
	case strings.Contains(family, "korean") || strings.Contains(family, "un dotum"):
		familyRank = 5
	}
	if len(preferredFamily) > 0 && family == preferredFamily[0] {
		familyRank = -1
	}
	weight := int(candidate.Aspect.Weight)
	if wantBold {
		return familyRank*1000 + absInt(weight-700)
	}
	return familyRank*1000 + absInt(weight-400)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (f *faceSet) close() {
	for _, face := range f.faces {
		_ = face.Close()
	}
}

func (f *faceSet) face(size int, bold bool) font.Face {
	key := faceKey{size: size, bold: bold}
	if face := f.faces[key]; face != nil {
		return face
	}
	fontFile := f.regular
	if bold {
		fontFile = f.bold
	}
	face, err := opentype.NewFace(fontFile, &opentype.FaceOptions{
		Size: float64(size), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	f.faces[key] = face
	return face
}

func drawText(dst *image.NRGBA, faces *faceSet, text string, x, baseline, size int, ink color.Color, bold bool) {
	face := faces.face(size, bold)
	if face == nil || text == "" {
		return
	}
	d := font.Drawer{Dst: dst, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(x, baseline)}
	d.DrawString(text)
}

func measure(faces *faceSet, text string, size int, bold bool) int {
	face := faces.face(size, bold)
	if face == nil {
		return 0
	}
	return font.MeasureString(face, text).Ceil()
}

func drawCentered(dst *image.NRGBA, faces *faceSet, text string, x, baseline, width, size int, ink color.Color, bold bool) {
	textWidth := measure(faces, text, size, bold)
	drawText(dst, faces, text, x+(width-textWidth)/2, baseline, size, ink, bold)
}

func drawClipped(dst *image.NRGBA, faces *faceSet, text string, x, baseline, width, size int, ink color.Color, bold bool) {
	if width <= 0 {
		return
	}
	if measure(faces, text, size, bold) <= width {
		drawText(dst, faces, text, x, baseline, size, ink, bold)
		return
	}
	const ellipsis = "…"
	limit := width - measure(faces, ellipsis, size, bold)
	var kept strings.Builder
	for len(text) > 0 {
		r, n := utf8.DecodeRuneInString(text)
		candidate := kept.String() + string(r)
		if measure(faces, candidate, size, bold) > limit {
			break
		}
		kept.WriteRune(r)
		text = text[n:]
	}
	drawText(dst, faces, kept.String()+ellipsis, x, baseline, size, ink, bold)
}

func drawBattery(dst *image.NRGBA, faces *faceSet, percent *int) {
	if percent == nil {
		return
	}
	value := max(0, min(100, *percent))
	const x, y = Width - 82, 15
	ink := color.NRGBA{R: 71, G: 85, B: 105, A: 255}
	fillRect(dst, x+2, y, x+20, y+14, ink)
	fillRect(dst, x+4, y+2, x+18, y+12, color.White)
	fillRect(dst, x+20, y+4, x+23, y+10, ink)
	if value > 0 {
		fillWidth := (14*value + 50) / 100
		fillRect(dst, x+4, y+2, x+4+fillWidth, y+12, ink)
	}
	drawText(dst, faces, fmt.Sprintf("%d%%", value), x+29, y+13, 16, color.NRGBA{R: 51, G: 65, B: 85, A: 255}, true)
}

func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, c color.Color) {
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > img.Bounds().Dx() {
		x1 = img.Bounds().Dx()
	}
	if y1 > img.Bounds().Dy() {
		y1 = img.Bounds().Dy()
	}
	if x0 >= x1 || y0 >= y1 {
		return
	}
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func startOfWeek(day time.Time, start string) time.Time {
	weekday := int(day.Weekday())
	first := 1
	if start == "sunday" {
		first = 0
	}
	delta := (7 + weekday - first) % 7
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).AddDate(0, 0, -delta)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func formatDate(day time.Time, locale string) string {
	if locale == "en" {
		return day.Format("01/02/2006")
	}
	return day.Format("2006. 01. 02.")
}

func formatDateTime(day time.Time, locale string) string {
	return formatDate(day, locale) + " " + day.Format("15:04")
}

func formatWeekday(day time.Time, locale string) string {
	if locale == "en" {
		return []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}[day.Weekday()]
	}
	return []string{"일", "월", "화", "수", "목", "금", "토"}[day.Weekday()]
}

func weekdayLabels(locale, start string) []string {
	ko := []string{"월", "화", "수", "목", "금", "토", "일"}
	en := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	if start == "sunday" {
		ko = []string{"일", "월", "화", "수", "목", "금", "토"}
		en = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	}
	if locale == "en" {
		return en
	}
	return ko
}

func eventLine(event Occurrence, loc *time.Location, locale string) string {
	title := event.Summary
	if title == "" {
		title = localized("제목 없음", "No title", locale)
	}
	if event.AllDay {
		return localized("종일 · ", "All-day · ", locale) + title
	}
	return event.Start.In(loc).Format("15:04") + "~" + event.End.In(loc).Format("15:04") + " " + title
}

func localized(ko, en, locale string) string {
	if locale == "en" {
		return en
	}
	return ko
}
