package log

import (
	"fmt"
	stdlog "log"
	"os"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelError Level = "ERROR"
)

var (
	logger     *stdlog.Logger
	loggerOnce sync.Once
	minLevel   = LevelInfo
)

// initLogger initializes the global logger to write to stderr with timestamps.
func initLogger() {
	loggerOnce.Do(func() {
		logger = stdlog.New(os.Stderr, "", stdlog.LstdFlags|stdlog.Lmicroseconds)
		// Default minimum level is INFO; can be made configurable later.
		minLevel = LevelInfo
	})
}

func SetLevel(l Level) {
	initLogger()
	minLevel = l
}

func Debug(msg string, kv ...any) {
	logWithLevel(LevelDebug, msg, kv...)
}

func Info(msg string, kv ...any) {
	logWithLevel(LevelInfo, msg, kv...)
}

func Error(msg string, err error, kv ...any) {
	// Prepend error into key-value list.
	extended := append([]any{"err", err}, kv...)
	logWithLevel(LevelError, msg, extended...)
}

func logWithLevel(level Level, msg string, kv ...any) {
	initLogger()
	if !enabled(level) {
		return
	}

	ts := time.Now().Format(time.RFC3339Nano)

	// Basic line format:
	// 2025-01-01T00:00:00Z [LEVEL] msg key=value ...
	line := ts + " [" + string(level) + "] " + msg

	// Append structured key-value pairs.
	if len(kv) > 0 {
		line += formatKVs(kv...)
	}

	logger.Println(line)
}

func enabled(level Level) bool {
	switch minLevel {
	case LevelDebug:
		return true
	case LevelInfo:
		return level == LevelInfo || level == LevelError
	case LevelError:
		return level == LevelError
	default:
		return true
	}
}

func formatKVs(kv ...any) string {
	out := ""
	// Expect kv as pairs: key, value, key, value, ...
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		val := kv[i+1]
		out += " " + key + "=" + safeSprint(val)
	}
	// If odd number of args, last one is ignored.
	return out
}

func safeSprint(v any) string {
	// Simpler and correct: rely on fmt.Sprint.
	return fmt.Sprint(v)
}
