package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

var (
	isJSONFormat bool
)

// Init initializes the global slog default logger based on format ("text" or "json")
// and verbose level. Returns the initialized *slog.Logger.
func Init(format string, verbose bool, out io.Writer) *slog.Logger {
	if out == nil {
		out = os.Stderr
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	isJSONFormat = (normalizedFormat == "json")

	var handler slog.Handler
	if isJSONFormat {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

// IsJSON returns true if the logger was configured with JSON format.
func IsJSON() bool {
	return isJSONFormat
}

// SetJSONForTesting allows unit tests to explicitly configure the JSON format state.
func SetJSONForTesting(isJSON bool) {
	isJSONFormat = isJSON
}
