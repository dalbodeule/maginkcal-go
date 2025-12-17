package ics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	appLog "epdcal/internal/log"
)

// Source represents a single ICS subscription source.
type Source struct {
	// ID is an internal identifier (e.g., config ICS ID).
	ID string
	// URL is the ICS endpoint.
	URL string
}

// FetchResult contains the outcome of fetching a single ICS source.
type FetchResult struct {
	Source    Source
	Body      []byte // ICS payload (either freshly fetched or from cache)
	FromCache bool   // true if we reused cached body due to 304
}

// cacheEntry holds HTTP cache metadata for a single ICS URL.
type cacheEntry struct {
	URL          string    `json:"url"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Fetcher is responsible for fetching ICS feeds with HTTP caching
// (ETag / Last-Modified) and disk-backed cache.
type Fetcher struct {
	client   *http.Client
	cacheDir string
}

// NewFetcher creates a new ICS Fetcher.
//
// cacheDir is the base directory where per-URL cache subdirectories and
// metadata will be stored. Example: "/var/lib/epdcal/ics-cache".
func NewFetcher(cacheDir string) *Fetcher {
	if cacheDir == "" {
		// Caller should set this explicitly; we fallback to a relative dir
		// so that development runs without root permissions.
		cacheDir = "./var/ics-cache"
	}
	return &Fetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		cacheDir: cacheDir,
	}
}

// FetchAll fetches all given sources and returns individual results.
// Errors for individual sources are logged and returned in the error slice.
//
// The returned slice of results will only contain entries for sources that
// successfully produced a body (either from network or cache).
func (f *Fetcher) FetchAll(ctx context.Context, sources []Source) ([]FetchResult, []error) {
	results := make([]FetchResult, 0, len(sources))
	errs := make([]error, 0)

	for _, src := range sources {
		res, err := f.FetchOne(ctx, src)
		if err != nil {
			errs = append(errs, err)
			appLog.Error("ics fetch failed", err, "id", src.ID, "url", redactURL(src.URL))
			continue
		}
		results = append(results, res)
	}

	return results, errs
}

// FetchOne fetches a single ICS source, honoring ETag and Last-Modified.
// It uses a disk cache under f.cacheDir keyed by a hash of the URL.
func (f *Fetcher) FetchOne(ctx context.Context, src Source) (FetchResult, error) {
	if src.URL == "" {
		return FetchResult{}, errors.New("source URL is empty")
	}

	cachePath, err := f.cachePathForURL(src.URL)
	if err != nil {
		return FetchResult{}, err
	}

	if err := os.MkdirAll(cachePath, 0o700); err != nil {
		return FetchResult{}, err
	}

	meta, _ := f.loadCacheMeta(cachePath)
	cachedBody, _ := f.loadCacheBody(cachePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return FetchResult{}, err
	}

	// Conditional headers from cache metadata.
	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	if meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", meta.LastModified)
	}

	appLog.Info("ics fetch start", "id", src.ID, "url", redactURL(src.URL))

	resp, err := f.client.Do(req)
	if err != nil {
		// Network error; if we have a cached body, fall back to it.
		if len(cachedBody) > 0 {
			appLog.Error("ics fetch network error, using cached body", err, "id", src.ID, "url", redactURL(src.URL))
			return FetchResult{
				Source:    src,
				Body:      cachedBody,
				FromCache: true,
			}, nil
		}
		return FetchResult{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Fresh content.
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return FetchResult{}, readErr
		}

		newMeta := cacheEntry{
			URL:          src.URL,
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			UpdatedAt:    time.Now().UTC(),
		}

		if err := f.saveCache(cachePath, newMeta, body); err != nil {
			// Log but still return the freshly fetched body.
			appLog.Error("ics cache save failed", err, "id", src.ID, "url", redactURL(src.URL))
		}

		appLog.Info("ics fetch success", "id", src.ID, "url", redactURL(src.URL), "status", resp.StatusCode, "from_cache", false)

		return FetchResult{
			Source:    src,
			Body:      body,
			FromCache: false,
		}, nil

	case http.StatusNotModified:
		// No change; use cached body if available.
		if len(cachedBody) == 0 {
			// 304 but no cached body: treat as error.
			return FetchResult{}, errors.New("received 304 Not Modified but no cached body available")
		}
		appLog.Info("ics fetch not modified; using cache", "id", src.ID, "url", redactURL(src.URL))
		return FetchResult{
			Source:    src,
			Body:      cachedBody,
			FromCache: true,
		}, nil

	default:
		// Non-OK status: if we have cached data, fall back to it.
		if len(cachedBody) > 0 {
			appLog.Error("ics fetch non-OK, using cached body", errors.New(resp.Status), "id", src.ID, "url", redactURL(src.URL), "status", resp.StatusCode)
			return FetchResult{
				Source:    src,
				Body:      cachedBody,
				FromCache: true,
			}, nil
		}
		return FetchResult{}, errors.New(resp.Status)
	}
}

func (f *Fetcher) cachePathForURL(url string) (string, error) {
	if url == "" {
		return "", errors.New("empty url")
	}
	sum := sha256.Sum256([]byte(url))
	// Use first 16 hex chars as directory name.
	dir := hex.EncodeToString(sum[:8])
	return filepath.Join(f.cacheDir, dir), nil
}

func (f *Fetcher) loadCacheMeta(cachePath string) (cacheEntry, error) {
	var meta cacheEntry
	metaFile := filepath.Join(cachePath, "meta.json")

	data, err := os.ReadFile(metaFile)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return cacheEntry{}, err
	}
	return meta, nil
}

func (f *Fetcher) loadCacheBody(cachePath string) ([]byte, error) {
	bodyFile := filepath.Join(cachePath, "body.ics")
	return os.ReadFile(bodyFile)
}

func (f *Fetcher) saveCache(cachePath string, meta cacheEntry, body []byte) error {
	metaFile := filepath.Join(cachePath, "meta.json")
	bodyFile := filepath.Join(cachePath, "body.ics")

	// Write body first so meta never points at missing body.
	if err := os.WriteFile(bodyFile, body, 0o600); err != nil {
		return err
	}

	meta.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(&meta, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(metaFile, data, 0o600); err != nil {
		return err
	}

	return nil
}

// redactURL hides sensitive parts of an ICS URL for logging purposes.
func redactURL(u string) string {
	// Very simple redaction to avoid logging query strings / paths in full.
	// Example:
	//   https://example.com/path/to/private.ics?token=abcd
	// -> https://example.com/...(redacted)
	const redactedSuffix = "/...(redacted)"

	// Find scheme separator.
	i := -1
	for idx := 0; idx+2 < len(u); idx++ {
		if u[idx:idx+3] == "://" {
			i = idx + 3
			break
		}
	}
	if i == -1 {
		return "ics://...(redacted)"
	}

	// Find next slash after host.
	j := i
	for j < len(u) && u[j] != '/' {
		j++
	}

	host := u[:j]
	return host + redactedSuffix
}
