package capture

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// Default capture parameters for the e-paper calendar.
// These should match the layout used by the /calendar page.
const (
	DefaultWidth      = 984
	DefaultHeight     = 1304
	DefaultTimeoutSec = 30
)

// CaptureOptions defines parameters for a Chromium-based screenshot capture.
type CaptureOptions struct {
	// URL to capture, e.g. "http://127.0.0.1:8080/calendar".
	URL string

	// OutputPath is where the PNG screenshot will be written, e.g.
	// "/var/lib/epdcal/preview.png".
	OutputPath string

	// Width and Height are the viewport dimensions in pixels. If zero,
	// DefaultWidth / DefaultHeight are used.
	Width  int
	Height int

	// Timeout bounds the entire capture operation. If zero, a sane default
	// (DefaultTimeoutSec) is used.
	Timeout time.Duration
}

// CaptureCalendarPNG launches (or attaches to) a headless Chromium instance
// via chromedp, navigates to opts.URL (typically /calendar), waits for the
// DOM to signal that rendering is complete, and then captures a PNG
// screenshot at the requested resolution.
//
// Rendering-complete condition:
//   - The /calendar root element exposes a data-ready attribute:
//     <div data-ready="true" ...>
//   - This function will wait until `[data-ready="true"]` is visible before
//     taking the screenshot.
//
// Note: This helper does NOT perform any NRGBA -> packed 1bpp conversion;
// that is left to the caller. The resulting PNG is a full-color screenshot.
func CaptureCalendarPNG(parentCtx context.Context, opts CaptureOptions) error {
	if opts.URL == "" {
		return fmt.Errorf("capture: URL is required")
	}
	if opts.OutputPath == "" {
		return fmt.Errorf("capture: OutputPath is required")
	}
	if opts.Width <= 0 {
		opts.Width = DefaultWidth
	}
	if opts.Height <= 0 {
		opts.Height = DefaultHeight
	}
	if opts.Timeout <= 0 {
		opts.Timeout = time.Duration(DefaultTimeoutSec) * time.Second
	}

	// Create a new chromedp context.
	ctx, cancel := chromedp.NewContext(parentCtx)
	defer cancel()

	// Apply timeout to the entire capture sequence.
	ctx, timeoutCancel := context.WithTimeout(ctx, opts.Timeout)
	defer timeoutCancel()

	var png []byte
	tasks := chromedp.Tasks{
		chromedp.EmulateViewport(int64(opts.Width), int64(opts.Height)),
		chromedp.Navigate(opts.URL),
		// Wait until /calendar signals that it has finished loading data
		// and rendering via data-ready="true".
		chromedp.WaitVisible(`[data-ready="true"]`, chromedp.ByQuery),
		// Small extra delay to allow final paints.
		chromedp.Sleep(500 * time.Millisecond),
		chromedp.FullScreenshot(&png, 100),
	}

	if err := chromedp.Run(ctx, tasks); err != nil {
		return fmt.Errorf("capture: chromedp run failed: %w", err)
	}

	if err := os.WriteFile(opts.OutputPath, png, 0o644); err != nil {
		return fmt.Errorf("capture: failed to write PNG: %w", err)
	}

	return nil
}
