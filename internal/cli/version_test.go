package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pandusatrianura/driftmock/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBuildInfo(t *testing.T) {
	info := cli.GetBuildInfo()
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.GoVersion)
	assert.NotEmpty(t, info.Platform)
}

func TestRunVersion_Text(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	err := cli.RunVersion(buf, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "DriftMock")
	assert.Contains(t, out, "runtime: go")
}

func TestRunVersion_JSON(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	err := cli.RunVersion(buf, true)
	require.NoError(t, err)

	var info cli.BuildInfo
	err = json.Unmarshal(buf.Bytes(), &info)
	require.NoError(t, err)

	assert.Equal(t, cli.Version, info.Version)
	assert.NotEmpty(t, info.GoVersion)
	assert.NotEmpty(t, info.Platform)
}
