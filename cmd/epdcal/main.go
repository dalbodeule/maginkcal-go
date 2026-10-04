package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"epdcal/internal/buildinfo"
	"epdcal/internal/config"
	"epdcal/internal/convert"
	"epdcal/internal/epd"
	appLog "epdcal/internal/log"
	"epdcal/internal/render"
	"epdcal/internal/web"
)

// flagConfig holds CLI flag values.
type flagConfig struct {
	configPath string
	listen     string
	once       bool
	renderOnly bool
	dump       bool
	debug      bool
}

func main() {
	appLog.Info("epdcal starting", "version", buildinfo.Version)

	// Parse CLI flags.
	flags := parseFlags()

	// Debug 모드에서는 기본 config 경로를 ./config.yaml 로 바꿔서
	// /etc 에 쓸 권한이 없는 개발 환경에서도 동작하게 한다.
	if flags.debug && flags.configPath == "/etc/epdcal/config.yaml" {
		flags.configPath = "./config.yaml"
	}

	// Load config (YAML with first-run creation + 0600 perms).
	conf, err := config.Load(flags.configPath)
	if err != nil {
		appLog.Error("failed to load config", err, "config_path", flags.configPath)
		os.Exit(1)
	}

	// CLI --listen overrides config file listen if provided.
	if flags.listen != "" {
		conf.Listen = flags.listen
	}
	if err := render.CheckFonts(conf.FontFamily); err != nil {
		appLog.Error("font check failed; epdcal cannot render the calendar", err)
		os.Exit(1)
	}

	appLog.Info("effective config",
		"config_path", flags.configPath,
		"listen", "http://"+conf.Listen,
		"timezone", conf.Timezone,
		"refresh_cron", conf.RefreshCron,
		"horizon_days", conf.HorizonDays,
		"show_all_day", conf.ShowAllDay,
		"ics_count", len(conf.ICS),
		"once", flags.once,
		"render_only", flags.renderOnly,
		"dump", flags.dump,
		"debug", flags.debug,
	)

	// Root context with cancellation on SIGINT/SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		appLog.Info("signal received, shutting down", "signal", sig.String())
		cancel()
	}()

	// Initialize EPD driver (C-based, via cgo) unless render-only.
	// NOTE: This requires CGO_ENABLED=1 and the C driver (DEV_Config.c,
	// EPD_12in48b.c) to be built/linked via internal/epd/epd_cgo.go.
	var epdDrv *epd.CDriver
	if !flags.renderOnly {
		d, err := epd.InitC()
		if err != nil {
			appLog.Error("failed to initialize C-based EPD driver; continuing in render-only mode", err)
		} else {
			appLog.Info("C-based epd driver initialized")
			epdDrv = d
		}
		defer func() {
			if epdDrv != nil {
				epdDrv.Sleep()
			}
		}()
	}

	server := web.NewServer(conf, flags.debug)
	server.SetConfigPath(flags.configPath)
	refresh := web.NewRefreshManager(ctx, func(ctx context.Context) error {
		active := server.Config()
		server.InvalidateEventsCache()
		return runRenderPipeline(ctx, active, flags, epdDrv)
	})
	defer refresh.WaitIdle()
	server.SetRefreshManager(refresh)
	// Bind before the initial render requests calendar data from the API.
	serverErrors, err := web.StartServer(ctx, server)
	if err != nil {
		appLog.Error("failed to start HTTP server", err)
		os.Exit(1)
	}
	go func() {
		if err := <-serverErrors; err != nil {
			appLog.Error("http server failed", err)
			cancel()
		}
	}()

	// Scheduler / single-run behavior.
	if flags.once {
		appLog.Info("running in once mode (single refresh cycle)")
		if _, err := refresh.RunScheduled(ctx); err != nil {
			appLog.Error("refresh/render failed in once mode", err)
			os.Exit(1)
		}

		appLog.Info("once mode completed; exiting")
		cancel()
		return
	}

	appLog.Info("starting periodic refresh loop (cron)",
		"refresh_cron", server.Config().RefreshCron,
	)

	// Initial immediate run.
	if _, err := refresh.RunScheduled(ctx); err != nil {
		appLog.Error("initial refresh/render failed", err)
	}

	if err := runScheduler(ctx, server, refresh); err != nil {
		appLog.Error("scheduler failed", err)
		os.Exit(1)
	}
	appLog.Info("context canceled; stopping cron scheduler")
	// Small delay for any future cleanup hooks (EPD sleep, etc.).
	time.Sleep(100 * time.Millisecond)
	appLog.Info("epdcal exiting")
	return
}

func runScheduler(ctx context.Context, server *web.Server, refresh *web.RefreshManager) error {
	for ctx.Err() == nil {
		cfg := server.Config()
		schedule, err := cron.ParseStandard(cfg.RefreshCron)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			appLog.Error("failed to load timezone, falling back to local", err, "timezone", cfg.Timezone)
			loc = time.Local
		}
		next := schedule.Next(time.Now().In(loc))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-server.ConfigChanged():
			timer.Stop()
			continue
		case <-timer.C:
			appLog.Info("scheduled refresh tick (cron)", "time", next.Format(time.RFC3339))
			if started, err := refresh.RunScheduled(ctx); !started {
				appLog.Info("scheduled refresh skipped; pipeline busy")
			} else if err != nil {
				appLog.Error("scheduled refresh/render failed", err)
			}
		}
	}
	return nil
}

func parseFlags() flagConfig {
	var cfg flagConfig

	flag.StringVar(&cfg.configPath, "config", "/etc/epdcal/config.yaml", "Path to config file")
	flag.StringVar(&cfg.listen, "listen", "", "HTTP listen address (overrides config if set)")
	flag.BoolVar(&cfg.once, "once", false, "Run one fetch+parse cycle and exit")
	flag.BoolVar(&cfg.renderOnly, "render-only", false, "Render only; do not touch display hardware")
	flag.BoolVar(&cfg.dump, "dump", false, "Write black.bin and red.bin debug artifacts")
	flag.BoolVar(&cfg.debug, "debug", false, "Debug mode: use ./config.yaml and ./cache instead of /etc and /var/lib")

	flag.Parse()

	return cfg
}

// runRenderPipeline renders the server's calendar data directly without a browser.
func runRenderPipeline(parentCtx context.Context, conf *config.Config, flags flagConfig, drv *epd.CDriver) error {
	const renderTimeout = 180 * time.Second
	ctx, cancel := context.WithTimeout(parentCtx, renderTimeout)
	defer cancel()

	outPath := "/var/lib/epdcal/preview.png"
	if flags.debug {
		outPath = "./cache/preview.png"
	}
	appLog.Info("starting internal calendar render", "output", outPath)

	apiURL := "http://" + loopbackAddress(conf.Listen)
	client := &http.Client{Timeout: 60 * time.Second}
	var events struct {
		Occurrences  []render.Occurrence `json:"occurrences"`
		HolidayDates []string            `json:"holiday_dates"`
		Timezone     string              `json:"display_timezone"`
		WeekStart    string              `json:"week_start"`
	}
	if err := getJSON(ctx, client, apiURL+"/api/events", conf.BasicAuth, &events); err != nil {
		return fmt.Errorf("render: load calendar events: %w", err)
	}

	var batteryInfo struct {
		Available bool `json:"available"`
		Percent   *int `json:"percent"`
	}
	if err := getJSON(ctx, client, apiURL+"/api/battery", conf.BasicAuth, &batteryInfo); err != nil {
		appLog.Error("battery status unavailable for render", err)
	}
	if !batteryInfo.Available {
		batteryInfo.Percent = nil
	}
	layout, err := config.ParseCalendarLayout(conf.LayoutJSON)
	if err != nil {
		return fmt.Errorf("render: invalid layout config: %w", err)
	}
	var weatherInfo struct {
		Available bool `json:"available"`
		Forecast  struct {
			Location    string   `json:"location"`
			Current     string   `json:"current"`
			CurrentTemp *float64 `json:"current_temp"`
		} `json:"forecast"`
	}
	var weatherPanel *render.Weather
	if conf.Weather.Enabled && layout.ShowWeather {
		if err := getJSON(ctx, client, apiURL+"/api/weather", conf.BasicAuth, &weatherInfo); err != nil || !weatherInfo.Available {
			if err == nil {
				err = fmt.Errorf("weather data unavailable")
			}
			appLog.Error("weather unavailable for render", err)
		} else {
			panel := render.Weather{Location: weatherInfo.Forecast.Location, Current: weatherInfo.Forecast.Current, CurrentTemp: weatherInfo.Forecast.CurrentTemp}
			weatherPanel = &panel
		}
	}
	assetDir := "/var/lib/epdcal/assets"
	if flags.debug {
		assetDir = "./assets"
	}

	img, err := render.RenderCalendar(render.Data{
		Now:          time.Now(),
		Locale:       conf.DefaultLocale,
		Timezone:     events.Timezone,
		WeekStart:    events.WeekStart,
		Occurrences:  events.Occurrences,
		HolidayDates: events.HolidayDates,
		Battery:      batteryInfo.Percent,
		Layout:       layout,
		AssetDir:     assetDir,
		FontFamily:   conf.FontFamily,
		Weather:      weatherPanel,
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("render: create output directory: %w", err)
	}
	file, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("render: create PNG: %w", err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		return fmt.Errorf("render: encode PNG: %w", err)
	}
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return fmt.Errorf("render: set PNG permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("render: close PNG: %w", err)
	}
	appLog.Info("internal calendar render completed", "output", outPath)

	black, red, err := convert.PackNRGBA(img, conf.Rotation)
	if err != nil {
		return err
	}

	if flags.dump {
		dir := filepath.Dir(outPath)
		for name, data := range map[string][]byte{"black.bin": black, "red.bin": red} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				return fmt.Errorf("render: write %s: %w", name, err)
			}
		}
	}

	if flags.renderOnly || drv == nil {
		return nil
	}

	appLog.Info("sending frame to EPD hardware")
	if err := drv.Display(black, red); err != nil {
		return err
	}

	appLog.Info("EPD frame update completed")
	return nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, auth *config.BasicAuthConfig, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned HTTP %d", req.URL.Path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func loopbackAddress(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
