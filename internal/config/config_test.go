package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHolidayPrefixDefaultsAndExplicitDisable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("timezone: Asia/Seoul\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || len(cfg.HolidayPrefixes) != 1 || cfg.HolidayPrefixes[0] != "쉬는 날" {
		t.Fatalf("default holiday prefixes = %+v, err = %v", cfg, err)
	}
	cfg.HolidayPrefixes = []string{}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil || len(reloaded.HolidayPrefixes) != 0 {
		t.Fatalf("holiday rule was not disabled: %+v, err = %v", reloaded, err)
	}
}
