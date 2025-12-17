package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"epdcal/internal/config"
	appLog "epdcal/internal/log"
)

// flagConfig holds CLI flag values before full config loading is implemented.
type flagConfig struct {
	configPath string
	listen     string
	once       bool
	renderOnly bool
	dump       bool
}

func main() {
	appLog.Info("epdcal starting", "version", "0.0.1-dev")

	// Parse CLI flags.
	flags := parseFlags()

	// Load config (currently returns defaults; will be extended in 2.2).
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
		"listen", conf.Listen,
		"timezone", conf.Timezone,
		"refresh_minutes", conf.RefreshMinutes,
		"horizon_days", conf.HorizonDays,
		"show_all_day", conf.ShowAllDay,
		"ics_count", len(conf.ICS),
		"once", flags.once,
		"render_only", flags.renderOnly,
		"dump", flags.dump,
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

	// TODO(2.1/2.2+): initialize scheduler, web server, ICS fetch/render pipeline.
	// - If flags.once == true, run single-shot pipeline and exit.
	// - Else, start background scheduler and web server.

	// For now, just block until context is canceled.
	<-ctx.Done()

	// Give some time for future cleanup hooks (EPD sleep, etc.).
	time.Sleep(100 * time.Millisecond)
	appLog.Info("epdcal exiting")
}

func parseFlags() flagConfig {
	var cfg flagConfig

	flag.StringVar(&cfg.configPath, "config", "/etc/epdcal/config.yaml", "Path to config file")
	flag.StringVar(&cfg.listen, "listen", "", "HTTP listen address (overrides config if set)")
	flag.BoolVar(&cfg.once, "once", false, "Run one fetch+render(+display) cycle and exit")
	flag.BoolVar(&cfg.renderOnly, "render-only", false, "Render only; do not touch display hardware")
	flag.BoolVar(&cfg.dump, "dump", false, "Dump debug artifacts (black.bin, red.bin, preview.png)")

	flag.Parse()

	return cfg
}
