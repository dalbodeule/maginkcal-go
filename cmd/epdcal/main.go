package main

import (
	"context"
	"errors"
	"flag"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"epdcal/internal/capture"
	"epdcal/internal/config"
	"epdcal/internal/convert"
	"epdcal/internal/epd"
	"epdcal/internal/ics"
	appLog "epdcal/internal/log"
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
	appLog.Info("epdcal starting", "version", "0.0.1-dev")

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

	// Start HTTP server in background.
	go func() {
		if err := web.StartServer(ctx, conf, flags.debug); err != nil {
			appLog.Error("http server failed", err)
			cancel()
		}
	}()

	// Signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		appLog.Info("signal received, shutting down", "signal", sig.String())
		cancel()
	}()

	// Initialize EPD SPI driver (unless render-only). This will only be
	// active on linux/arm builds where the epd SPI backend is available.
	var epdDrv *epd.Driver
	if !flags.renderOnly {
		d, err := epd.Init(ctx)
		if err != nil {
			appLog.Error("failed to initialize EPD driver; continuing in render-only mode", err)
		} else {
			if err := d.InitPanel(); err != nil {
				appLog.Error("failed to init EPD panel; continuing in render-only mode", err)
			} else {
				appLog.Info("epd driver initialized")
				epdDrv = d
			}
		}
		defer func() {
			if epdDrv != nil {
				if err := epdDrv.Sleep(); err != nil {
					appLog.Error("epd sleep failed", err)
				}
				if err := epdDrv.Close(); err != nil {
					appLog.Error("epd close failed", err)
				}
			}
		}()
	}

	// Scheduler / single-run behavior.
	if flags.once {
		appLog.Info("running in once mode (single refresh cycle)")
		if err := runRefreshCycle(ctx, conf, flags.debug); err != nil {
			appLog.Error("refresh cycle failed in once mode", err)
			os.Exit(1)
		}

		// 파이프라인의 일부로 /calendar 페이지를 Chromium으로 캡처해서
		// preview.png를 생성하고, PNG → packed plane 변환 후 EPD에 출력한다.
		// once 모드에서는 캡처/디스플레이 실패 시 프로세스를 종료하여
		// 문제를 빠르게 드러내도록 한다.
		if err := runCapturePipeline(ctx, conf, flags, epdDrv); err != nil {
			appLog.Error("capture/display pipeline failed in once mode", err)
			os.Exit(1)
		}

		appLog.Info("once mode completed; exiting")
		return
	}

	appLog.Info("starting periodic refresh loop (cron)",
		"refresh_cron", conf.RefreshCron,
	)

	// Initial immediate run.
	if err := runRefreshCycle(ctx, conf, flags.debug); err != nil {
		appLog.Error("initial refresh cycle failed", err)
	} else {
		// 주기 루프에서도 매 refresh 이후에 /calendar를 Chromium으로 캡처하여
		// preview.png를 최신 상태로 유지한다. 캡처 실패는 치명적이지 않으므로
		// 에러만 로그에 남기고 루프는 계속 돈다.
		if err := runCapturePipeline(ctx, conf, flags, epdDrv); err != nil {
			appLog.Error("chromium capture/display failed after initial refresh", err)
		}
	}

	// Use a cron-style scheduler for periodic refresh instead of a fixed ticker.
	// This allows true "wall-clock aligned" schedules (e.g. */15 * * * *) and
	// more complex patterns in the future.
	loc, err := time.LoadLocation(conf.Timezone)
	if err != nil {
		appLog.Error("failed to load timezone, falling back to local", err, "timezone", conf.Timezone)
		loc = time.Local
	}

	c := cron.New(cron.WithLocation(loc))

	_, err = c.AddFunc(conf.RefreshCron, func() {
		select {
		case <-ctx.Done():
			// Context canceled; do not start new work.
			return
		default:
		}

		now := time.Now().In(loc)
		appLog.Info("scheduled refresh tick (cron)", "time", now.Format(time.RFC3339))

		if err := runRefreshCycle(ctx, conf, flags.debug); err != nil {
			appLog.Error("scheduled refresh cycle failed", err)
			return
		}
		if err := runCapturePipeline(ctx, conf, flags, epdDrv); err != nil {
			appLog.Error("chromium capture/display failed after scheduled refresh", err)
		}
	})
	if err != nil {
		appLog.Error("failed to add cron schedule", err, "refresh_cron", conf.RefreshCron)
		os.Exit(1)
	}

	c.Start()
	defer c.Stop()

	// Block until context is canceled (SIGINT/SIGTERM).
	<-ctx.Done()
	appLog.Info("context canceled; stopping cron scheduler")
	// Small delay for any future cleanup hooks (EPD sleep, etc.).
	time.Sleep(100 * time.Millisecond)
	appLog.Info("epdcal exiting")
	return
}

func parseFlags() flagConfig {
	var cfg flagConfig

	flag.StringVar(&cfg.configPath, "config", "/etc/epdcal/config.yaml", "Path to config file")
	flag.StringVar(&cfg.listen, "listen", "", "HTTP listen address (overrides config if set)")
	flag.BoolVar(&cfg.once, "once", false, "Run one fetch+parse cycle and exit")
	flag.BoolVar(&cfg.renderOnly, "render-only", false, "Render only; do not touch display hardware (reserved)")
	flag.BoolVar(&cfg.dump, "dump", false, "Dump debug artifacts (black.bin, red.bin, preview.png) (reserved)")
	flag.BoolVar(&cfg.debug, "debug", false, "Debug mode: use ./config.yaml and ./cache instead of /etc and /var/lib")

	flag.Parse()

	return cfg
}

// runRefreshCycle performs a single ICS fetch+parse cycle for all configured
// ICS sources. For now it only logs counts; later this will feed recurrence
// expansion, rendering, and EPD display.
func runRefreshCycle(parentCtx context.Context, conf *config.Config, debug bool) error {
	startTime := time.Now()
	appLog.Info("refresh cycle start", "start_time", startTime.Format(time.RFC3339), "ics_count", len(conf.ICS), "debug", debug)

	if len(conf.ICS) == 0 {
		appLog.Info("no ICS sources configured; skipping refresh cycle")
		return nil
	}

	// Derive a cycle-scoped context with timeout to avoid hanging forever
	// on slow/unresponsive ICS servers.
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()

	// Build source list from config.
	sources := make([]ics.Source, 0, len(conf.ICS))
	for _, csrc := range conf.ICS {
		if csrc.URL == "" {
			continue
		}
		id := csrc.ID
		if id == "" {
			// Fallback to name or short URL if ID is missing.
			if csrc.Name != "" {
				id = csrc.Name
			} else {
				id = csrc.URL
			}
		}
		sources = append(sources, ics.Source{
			ID:  id,
			URL: csrc.URL,
		})
	}

	if len(sources) == 0 {
		appLog.Info("no valid ICS sources (all missing URLs); skipping refresh cycle")
		return nil
	}

	// cacheDir 선택:
	// - 기본: /var/lib/epdcal/ics-cache
	// - debug 모드: ./cache/ics-cache (개발 환경에서 root 없이 사용)
	const defaultCacheDir = "/var/lib/epdcal/ics-cache"
	cacheDir := defaultCacheDir
	if debug {
		cacheDir = "./cache/ics-cache"
	}
	fetcher := ics.NewFetcher(cacheDir)

	// Fetch all ICS feeds.
	fetchResults, fetchErrs := fetcher.FetchAll(ctx, sources)
	if len(fetchErrs) > 0 {
		appLog.Error("one or more ICS fetches failed", errorsAggregate(fetchErrs), "error_count", len(fetchErrs))
	}

	totalParsedEvents := 0

	for _, res := range fetchResults {
		parsed, err := ics.ParseICS(res.Source, res.Body)
		if err != nil {
			appLog.Error("ics parse for source failed", err, "id", res.Source.ID, "url", icsRedactedURL(res.Source))
			continue
		}
		totalParsedEvents += len(parsed)

		appLog.Info("ics source processed",
			"id", res.Source.ID,
			"from_cache", res.FromCache,
			"event_count", len(parsed),
		)

		// TODO: store parsed events into a shared model/cache so that
		// rendering/scheduling can consume them.
	}

	elapsed := time.Since(startTime)
	appLog.Info("refresh cycle completed",
		"duration", elapsed.String(),
		"parsed_event_total", totalParsedEvents,
	)

	return nil
}

// runCapturePipeline performs a Chromium-based PNG capture of the
// /calendar page using the capture.CaptureCalendarPNG helper.
//
//   - 주기적인 refresh 파이프라인에서 사용되어, 항상 최신 캘린더 뷰를
//     preview.png 로 유지한다.
//   - also used in once mode to validate that the whole stack (web + capture)
//     is working end-to-end.
//
// In debug mode it writes to ./cache/preview.png, otherwise to
// /var/lib/epdcal/preview.png.
func runCapturePipeline(parentCtx context.Context, conf *config.Config, flags flagConfig, drv *epd.Driver) error {
	// Derive a short-lived context for the capture operation.
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()

	url := "http://" + conf.Listen + "/calendar"

	outPath := "/var/lib/epdcal/preview.png"
	if flags.debug {
		outPath = "./cache/preview.png"
	}

	appLog.Info("starting chromium capture",
		"url", url,
		"output", outPath,
	)

	opts := capture.CaptureOptions{
		URL:        url,
		OutputPath: outPath,
		Width:      0, // use defaults
		Height:     0,
		Timeout:    120 * time.Second,
	}

	if err := capture.CaptureCalendarPNG(ctx, opts); err != nil {
		return err
	}

	appLog.Info("chromium capture completed", "output", outPath)

	// Load the captured PNG, convert to NRGBA, then pack into black/red planes.
	f, err := os.Open(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return err
	}

	var nrgba *image.NRGBA
	if v, ok := img.(*image.NRGBA); ok {
		nrgba = v
	} else {
		// Convert to NRGBA via draw.Draw.
		bounds := img.Bounds()
		tmp := image.NewNRGBA(bounds)
		draw.Draw(tmp, bounds, img, bounds.Min, draw.Src)
		nrgba = tmp
	}

	black, red, err := convert.PackNRGBA(nrgba, conf.Rotation)
	if err != nil {
		return err
	}

	// If --dump is enabled, write black.bin and red.bin alongside preview.png.
	if flags.dump {
		dir := filepath.Dir(outPath)
		blackPath := filepath.Join(dir, "black.bin")
		redPath := filepath.Join(dir, "red.bin")

		if err := os.WriteFile(blackPath, black, 0o644); err != nil {
			appLog.Error("failed to write black.bin", err, "path", blackPath)
		} else {
			appLog.Info("wrote black.bin", "path", blackPath)
		}
		if err := os.WriteFile(redPath, red, 0o644); err != nil {
			appLog.Error("failed to write red.bin", err, "path", redPath)
		} else {
			appLog.Info("wrote red.bin", "path", redPath)
		}
	}

	// If render-only or no EPD driver is available, stop after generating planes.
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

// errorsAggregate creates a simple aggregated error message for logging.
func errorsAggregate(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	// Simple join; no need for full multi-error type at this stage.
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return errors.New(b.String())
}

// icsRedactedURL is a tiny wrapper to avoid leaking actual URLs from main.
func icsRedactedURL(src ics.Source) string {
	// We intentionally do not log the actual URL from main.
	return "ics://source(" + src.ID + ")"
}
