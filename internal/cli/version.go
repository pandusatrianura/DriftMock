package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// Version holds the semantic version of DriftMock (can be injected via -ldflags).
	Version = "v0.1.0-dev"

	// Commit holds the git commit hash (can be injected via -ldflags).
	Commit = "none"

	// BuildDate holds the compilation timestamp (can be injected via -ldflags).
	BuildDate = "unknown"
)

// BuildInfo encapsulates full runtime and build metadata for DriftMock.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetBuildInfo retrieves the resolved build metadata.
func GetBuildInfo() BuildInfo {
	commit := Commit
	buildDate := BuildDate

	if commit == "none" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
					if len(commit) > 7 {
						commit = commit[:7]
					}
				}
				if setting.Key == "vcs.time" {
					buildDate = setting.Value
				}
			}
		}
	}

	return BuildInfo{
		Version:   Version,
		Commit:    commit,
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

var versionJSON bool

// NewVersionCmd creates the Cobra version subcommand.
func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Display DriftMock version and build information",
		Long:  "Prints the version, commit hash, build date, Go runtime, and platform architecture.",
		Example: `  # Human readable output
  driftmock version

  # JSON formatted output
  driftmock version --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunVersion(cmd.OutOrStdout(), versionJSON)
		},
	}

	cmd.Flags().BoolVar(&versionJSON, "json", false, "output version metadata in JSON format")
	return cmd
}

func init() {
	RootCmd.AddCommand(NewVersionCmd())
}

// RunVersion outputs build metadata to w in either plain text or JSON format.
func RunVersion(w io.Writer, asJSON bool) error {
	info := GetBuildInfo()

	if asJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}

	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	fmt.Fprintf(w, "%s %s (commit: %s, built: %s, runtime: %s, platform: %s)\n",
		cyan("DriftMock"),
		green(info.Version),
		yellow(info.Commit),
		info.BuildDate,
		info.GoVersion,
		info.Platform,
	)
	return nil
}
