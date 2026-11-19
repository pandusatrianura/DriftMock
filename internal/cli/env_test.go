package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnv_GlobalFlags(t *testing.T) {
	origFormat := logFormat
	origVerbose := verbose
	defer func() {
		logFormat = origFormat
		verbose = origVerbose
		_ = os.Unsetenv("DRIFTMOCK_LOG_FORMAT")
		_ = os.Unsetenv("DRIFTMOCK_VERBOSE")
	}()

	cmd := NewVersionCmd()

	// 1. Env vars populate flags when flags are unchanged
	_ = os.Setenv("DRIFTMOCK_LOG_FORMAT", "json")
	_ = os.Setenv("DRIFTMOCK_VERBOSE", "true")
	applyGlobalEnvVars(cmd)
	assert.Equal(t, "json", logFormat)
	assert.True(t, verbose)

	// 2. Boolean parsing variants (1, yes, on, false)
	assert.True(t, getEnvBool("DRIFTMOCK_VERBOSE", false))
	_ = os.Setenv("DRIFTMOCK_VERBOSE", "1")
	assert.True(t, getEnvBool("DRIFTMOCK_VERBOSE", false))
	_ = os.Setenv("DRIFTMOCK_VERBOSE", "yes")
	assert.True(t, getEnvBool("DRIFTMOCK_VERBOSE", false))
	_ = os.Setenv("DRIFTMOCK_VERBOSE", "false")
	assert.False(t, getEnvBool("DRIFTMOCK_VERBOSE", true))
}

func TestEnv_RecordOptions(t *testing.T) {
	defer func() {
		_ = os.Unsetenv("DRIFTMOCK_TARGET")
		_ = os.Unsetenv("DRIFTMOCK_PORT")
		_ = os.Unsetenv("DRIFTMOCK_STUBS_DIR")
	}()

	cmd := NewRecordCmd()
	var opts RecordOptions

	_ = os.Setenv("DRIFTMOCK_TARGET", "https://env.staging.example.com")
	_ = os.Setenv("DRIFTMOCK_PORT", "9090")
	_ = os.Setenv("DRIFTMOCK_STUBS_DIR", "/tmp/env-stubs")

	applyRecordEnvVars(cmd, &opts)
	assert.Equal(t, "https://env.staging.example.com", opts.Target)
	assert.Equal(t, 9090, opts.Port)
	assert.Equal(t, "/tmp/env-stubs", opts.OutDir)
}

func TestEnv_ReplayOptions(t *testing.T) {
	defer func() {
		_ = os.Unsetenv("DRIFTMOCK_PORT")
		_ = os.Unsetenv("DRIFTMOCK_STUBS_DIR")
		_ = os.Unsetenv("DRIFTMOCK_SIMULATE_DELAY")
		_ = os.Unsetenv("DRIFTMOCK_LATENCY_MS")
	}()

	cmd := NewReplayCmd()
	var opts ReplayCommandOptions

	_ = os.Setenv("DRIFTMOCK_PORT", "7070")
	_ = os.Setenv("DRIFTMOCK_STUBS_DIR", "/custom/stubs")
	_ = os.Setenv("DRIFTMOCK_SIMULATE_DELAY", "true")
	_ = os.Setenv("DRIFTMOCK_LATENCY_MS", "350")

	applyReplayEnvVars(cmd, &opts)
	assert.Equal(t, 7070, opts.Port)
	assert.Equal(t, "/custom/stubs", opts.StubsDir)
	assert.True(t, opts.SimulateDelay)
	assert.Equal(t, 350, opts.LatencyMs)
}

func TestEnv_CheckOptions(t *testing.T) {
	defer func() {
		_ = os.Unsetenv("DRIFTMOCK_TARGET")
		_ = os.Unsetenv("DRIFTMOCK_STUBS_DIR")
		_ = os.Unsetenv("DRIFTMOCK_STRICT")
		_ = os.Unsetenv("DRIFTMOCK_IGNORE_VOLATILE")
		_ = os.Unsetenv("DRIFTMOCK_TIMEOUT")
	}()

	cmd := NewCheckCmd()
	var opts CheckOptions

	_ = os.Setenv("DRIFTMOCK_TARGET", "https://staging.acme.corp")
	_ = os.Setenv("DRIFTMOCK_STUBS_DIR", "./mocks")
	_ = os.Setenv("DRIFTMOCK_STRICT", "false")
	_ = os.Setenv("DRIFTMOCK_IGNORE_VOLATILE", "false")
	_ = os.Setenv("DRIFTMOCK_TIMEOUT", "25")

	applyCheckEnvVars(cmd, &opts)
	assert.Equal(t, "https://staging.acme.corp", opts.Target)
	assert.Equal(t, "./mocks", opts.StubsDir)
	assert.False(t, opts.Strict)
	assert.False(t, opts.IgnoreVolatile)
	assert.Equal(t, 25, opts.TimeoutSeconds)
}

func TestEnv_PreRunE_RecordAndCheckValidation(t *testing.T) {
	defer func() {
		_ = os.Unsetenv("DRIFTMOCK_TARGET")
	}()

	// 1. Record without target and without env fails PreRunE
	_ = os.Unsetenv("DRIFTMOCK_TARGET")
	recordCmd := NewRecordCmd()
	err := recordCmd.PreRunE(recordCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target URL is required")

	// 2. Record with DRIFTMOCK_TARGET passes PreRunE
	_ = os.Setenv("DRIFTMOCK_TARGET", "https://api.example.com")
	recordCmd2 := NewRecordCmd()
	err = recordCmd2.PreRunE(recordCmd2, []string{})
	require.NoError(t, err)

	// 3. Check without target and without env fails PreRunE
	_ = os.Unsetenv("DRIFTMOCK_TARGET")
	checkCmd := NewCheckCmd()
	err = checkCmd.PreRunE(checkCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target URL is required")

	// 4. Check with DRIFTMOCK_TARGET passes PreRunE
	_ = os.Setenv("DRIFTMOCK_TARGET", "https://api.example.com")
	checkCmd2 := NewCheckCmd()
	err = checkCmd2.PreRunE(checkCmd2, []string{})
	require.NoError(t, err)
}
