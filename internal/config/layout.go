package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// CalendarLayout is the supported, versionable customization surface for the
// built-in renderer. It intentionally excludes arbitrary HTML/CSS/scripts.
type CalendarLayout struct {
	ShowWeather     bool           `json:"show_weather"`
	ShowBattery     bool           `json:"show_battery"`
	ShowEventTimes  bool           `json:"show_event_times"`
	ShowEmptyDays   bool           `json:"show_empty_days"`
	MaxEventsPerDay int            `json:"max_events_per_day"`
	HeaderFontSize  int            `json:"header_font_size"`
	DateFontSize    int            `json:"date_font_size"`
	EventFontSize   int            `json:"event_font_size"`
	GridTop         int            `json:"grid_top"`
	Assets          []AssetOverlay `json:"assets"`
}

type AssetOverlay struct {
	File   string `json:"file"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func DefaultCalendarLayout() CalendarLayout {
	return CalendarLayout{ShowWeather: true, ShowBattery: true, ShowEventTimes: true, ShowEmptyDays: true, MaxEventsPerDay: 3, HeaderFontSize: 36, DateFontSize: 16, EventFontSize: 12, GridTop: 179, Assets: []AssetOverlay{}}
}

// DefaultCalendarLayoutJSON returns the initial editable layout in a
// human-friendly format. It is persisted with newly-created configurations.
func DefaultCalendarLayoutJSON() string {
	data, _ := json.MarshalIndent(DefaultCalendarLayout(), "", "  ")
	return string(data)
}

func ParseCalendarLayout(raw string) (CalendarLayout, error) {
	layout := DefaultCalendarLayout()
	if strings.TrimSpace(raw) == "" {
		return layout, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var input struct {
		ShowWeather     *bool           `json:"show_weather"`
		WeatherDays     *int            `json:"weather_days"` // Accepted for old configs; the renderer now shows a compact current-conditions line.
		ShowBattery     *bool           `json:"show_battery"`
		ShowEventTimes  *bool           `json:"show_event_times"`
		ShowEmptyDays   *bool           `json:"show_empty_days"`
		MaxEventsPerDay *int            `json:"max_events_per_day"`
		HeaderFontSize  *int            `json:"header_font_size"`
		DateFontSize    *int            `json:"date_font_size"`
		EventFontSize   *int            `json:"event_font_size"`
		GridTop         *int            `json:"grid_top"`
		Assets          *[]AssetOverlay `json:"assets"`
	}
	if err := dec.Decode(&input); err != nil {
		return layout, fmt.Errorf("invalid layout JSON: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return layout, errors.New("layout JSON must contain one object")
	}
	if input.ShowBattery != nil {
		layout.ShowBattery = *input.ShowBattery
	}
	if input.ShowWeather != nil {
		layout.ShowWeather = *input.ShowWeather
	}
	if input.ShowEventTimes != nil {
		layout.ShowEventTimes = *input.ShowEventTimes
	}
	if input.ShowEmptyDays != nil {
		layout.ShowEmptyDays = *input.ShowEmptyDays
	}
	if input.MaxEventsPerDay != nil {
		layout.MaxEventsPerDay = *input.MaxEventsPerDay
	}
	if input.HeaderFontSize != nil {
		layout.HeaderFontSize = *input.HeaderFontSize
	}
	if input.DateFontSize != nil {
		layout.DateFontSize = *input.DateFontSize
	}
	if input.EventFontSize != nil {
		layout.EventFontSize = *input.EventFontSize
	}
	if input.GridTop != nil {
		layout.GridTop = *input.GridTop
	}
	if input.Assets != nil {
		layout.Assets = *input.Assets
	}
	if layout.MaxEventsPerDay < 1 || layout.MaxEventsPerDay > 8 {
		return layout, errors.New("max_events_per_day must be between 1 and 8")
	}
	if layout.HeaderFontSize < 12 || layout.HeaderFontSize > 64 || layout.DateFontSize < 8 || layout.DateFontSize > 32 || layout.EventFontSize < 8 || layout.EventFontSize > 24 {
		return layout, errors.New("layout font sizes are outside supported bounds")
	}
	if layout.GridTop < 150 || layout.GridTop > 700 {
		return layout, errors.New("grid_top must be between 150 and 700")
	}
	if len(layout.Assets) > 8 {
		return layout, errors.New("at most 8 asset overlays are allowed")
	}
	for _, asset := range layout.Assets {
		if asset.File == "" || filepath.Base(asset.File) != asset.File || strings.ContainsAny(asset.File, `/\\`) {
			return layout, errors.New("asset file must be a filename inside the assets directory")
		}
		ext := strings.ToLower(filepath.Ext(asset.File))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			return layout, errors.New("assets must be PNG or JPEG images")
		}
		if asset.X < 0 || asset.Y < 0 || asset.X >= 984 || asset.Y >= 1304 || asset.Width < 1 || asset.Height < 1 || asset.Width > 984 || asset.Height > 1304 || asset.X+asset.Width > 984 || asset.Y+asset.Height > 1304 {
			return layout, errors.New("asset overlay position or size is outside the display")
		}
	}
	return layout, nil
}
