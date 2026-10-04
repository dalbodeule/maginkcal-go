package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseCalendarLayoutDefaultsAndRejectsUnsafeAssets(t *testing.T) {
	layout, err := ParseCalendarLayout("")
	if err != nil || layout.GridTop != 179 || layout.MaxEventsPerDay != 3 {
		t.Fatalf("layout=%+v err=%v", layout, err)
	}
	if _, err := ParseCalendarLayout(`{"show_battery":true,"show_event_times":true,"show_empty_days":true,"max_events_per_day":3,"header_font_size":36,"date_font_size":16,"event_font_size":12,"grid_top":179,"assets":[{"file":"../secret.png","x":0,"y":0,"width":20,"height":20}]}`); err == nil {
		t.Fatal("path traversal asset accepted")
	}
	if _, err := ParseCalendarLayout(`{} {}`); err == nil {
		t.Fatal("multiple JSON values accepted")
	}
	if _, err := ParseCalendarLayout(`{"weather_days":5}`); err != nil {
		t.Fatalf("legacy weather_days field should remain compatible: %v", err)
	}
}

func TestInstalledConfigSampleContainsDefaultLayoutJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "systemd", "config.yaml.sample"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	layout, err := ParseCalendarLayout(cfg.LayoutJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(layout, DefaultCalendarLayout()) {
		t.Fatalf("sample layout = %+v, want default %+v", layout, DefaultCalendarLayout())
	}
}

func TestDefaultConfigIncludesCompleteLayoutJSON(t *testing.T) {
	cfg := DefaultConfig()
	layout, err := ParseCalendarLayout(cfg.LayoutJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(layout, DefaultCalendarLayout()) {
		t.Fatalf("default layout = %+v, want %+v", layout, DefaultCalendarLayout())
	}

	cfg.LayoutJSON = "  "
	cfg.Normalize()
	if _, err := ParseCalendarLayout(cfg.LayoutJSON); err != nil {
		t.Fatalf("normalized default layout is invalid: %v", err)
	}
}

func TestLoadCreatesConfigWithDefaultLayoutJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCalendarLayout(cfg.LayoutJSON); err != nil {
		t.Fatalf("loaded default layout is invalid: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Config
	if err := yaml.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCalendarLayout(persisted.LayoutJSON); err != nil {
		t.Fatalf("persisted default layout is invalid: %v", err)
	}
}

func TestSaveNormalizesBlankLayoutJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := DefaultConfig()
	cfg.LayoutJSON = " \n\t "
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ParseCalendarLayout(saved.LayoutJSON)
	if err != nil {
		t.Fatalf("blank layout was not normalized: %v", err)
	}
	if !reflect.DeepEqual(layout, DefaultCalendarLayout()) {
		t.Fatalf("saved layout = %+v, want default %+v", layout, DefaultCalendarLayout())
	}
}
