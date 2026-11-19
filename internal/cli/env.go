package cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// getEnv returns the environment variable value or fallback if unset or whitespace-only.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

// getEnvInt parses an integer environment variable or returns fallback if unset/invalid.
func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return parsed
		}
	}
	return fallback
}

// getEnvBool parses a boolean environment variable (true, 1, yes, on).
func getEnvBool(key string, fallback bool) bool {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "true" || lower == "1" || lower == "yes" || lower == "on"
	}
	return fallback
}

// applyGlobalEnvVars merges environment variables into persistent global flags.
func applyGlobalEnvVars(cmd *cobra.Command) {
	if !cmd.Flags().Changed("log-format") {
		if val := getEnv("DRIFTMOCK_LOG_FORMAT", ""); val != "" {
			logFormat = val
		}
	}
	if !cmd.Flags().Changed("verbose") {
		if _, ok := os.LookupEnv("DRIFTMOCK_VERBOSE"); ok {
			verbose = getEnvBool("DRIFTMOCK_VERBOSE", verbose)
		}
	}
}

// applyRecordEnvVars applies environment variables to record options if flags weren't explicitly changed.
func applyRecordEnvVars(cmd *cobra.Command, opts *RecordOptions) {
	if !cmd.Flags().Changed("target") {
		if val := getEnv("DRIFTMOCK_TARGET", ""); val != "" {
			opts.Target = val
		}
	}
	if !cmd.Flags().Changed("port") {
		if port := getEnvInt("DRIFTMOCK_PORT", 0); port > 0 {
			opts.Port = port
		}
	}
	if !cmd.Flags().Changed("out") {
		if out := getEnv("DRIFTMOCK_STUBS_DIR", getEnv("DRIFTMOCK_OUT_DIR", "")); out != "" {
			opts.OutDir = out
		}
	}
}

// applyReplayEnvVars applies environment variables to replay options if flags weren't explicitly changed.
func applyReplayEnvVars(cmd *cobra.Command, opts *ReplayCommandOptions) {
	if !cmd.Flags().Changed("port") {
		if port := getEnvInt("DRIFTMOCK_PORT", 0); port > 0 {
			opts.Port = port
		}
	}
	if !cmd.Flags().Changed("stubs") {
		if stubs := getEnv("DRIFTMOCK_STUBS_DIR", getEnv("DRIFTMOCK_STUBS", "")); stubs != "" {
			opts.StubsDir = stubs
		}
	}
	if !cmd.Flags().Changed("simulate-delay") {
		if _, ok := os.LookupEnv("DRIFTMOCK_SIMULATE_DELAY"); ok {
			opts.SimulateDelay = getEnvBool("DRIFTMOCK_SIMULATE_DELAY", opts.SimulateDelay)
		}
	}
	if !cmd.Flags().Changed("latency") {
		if latency := getEnvInt("DRIFTMOCK_LATENCY_MS", getEnvInt("DRIFTMOCK_LATENCY", -1)); latency >= 0 {
			opts.LatencyMs = latency
		}
	}
}

// applyCheckEnvVars applies environment variables to check options if flags weren't explicitly changed.
func applyCheckEnvVars(cmd *cobra.Command, opts *CheckOptions) {
	if !cmd.Flags().Changed("target") {
		if val := getEnv("DRIFTMOCK_TARGET", ""); val != "" {
			opts.Target = val
		}
	}
	if !cmd.Flags().Changed("stubs") {
		if stubs := getEnv("DRIFTMOCK_STUBS_DIR", getEnv("DRIFTMOCK_STUBS", "")); stubs != "" {
			opts.StubsDir = stubs
		}
	}
	if !cmd.Flags().Changed("strict") {
		if _, ok := os.LookupEnv("DRIFTMOCK_STRICT"); ok {
			opts.Strict = getEnvBool("DRIFTMOCK_STRICT", opts.Strict)
		}
	}
	if !cmd.Flags().Changed("ignore-volatile") {
		if _, ok := os.LookupEnv("DRIFTMOCK_IGNORE_VOLATILE"); ok {
			opts.IgnoreVolatile = getEnvBool("DRIFTMOCK_IGNORE_VOLATILE", opts.IgnoreVolatile)
		}
	}
	if !cmd.Flags().Changed("timeout") {
		if timeout := getEnvInt("DRIFTMOCK_TIMEOUT", 0); timeout > 0 {
			opts.TimeoutSeconds = timeout
		}
	}
}
