package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_JSON(t *testing.T) {
	buf := &bytes.Buffer{}
	l := logger.Init("json", false, buf)
	require.NotNil(t, l)
	assert.True(t, logger.IsJSON())

	slog.Info("test structured log", "key", "value", "count", 42)

	output := buf.String()
	assert.NotEmpty(t, output)

	var entry map[string]any
	err := json.Unmarshal([]byte(output), &entry)
	require.NoError(t, err, "output should be valid JSON: %s", output)
	assert.Equal(t, "test structured log", entry["msg"])
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "value", entry["key"])
	assert.Equal(t, float64(42), entry["count"])
}

func TestInit_Text(t *testing.T) {
	buf := &bytes.Buffer{}
	l := logger.Init("text", true, buf)
	require.NotNil(t, l)
	assert.False(t, logger.IsJSON())

	slog.Debug("debug message", "service", "driftmock")

	output := buf.String()
	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, "service=driftmock")
}
