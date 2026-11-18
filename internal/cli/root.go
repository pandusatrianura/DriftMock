package cli

import (
	"fmt"
	"os"

	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/spf13/cobra"
)

var (
	verbose   bool
	logFormat string
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:     "driftmock",
	Version: Version,
	Short:   "Zero-effort live traffic recording, instant offline API mocking, and contract drift detection.",
	Long: `DriftMock is a high-performance transparent reverse proxy between client applications
(frontend web, mobile apps, QA test runner) and backend staging/development environments.

Modes:
  record  - Intercept live staging traffic, sanitize secrets, and generate Git-committable JSON mock stubs.
  replay  - Serve recorded stubs offline with deterministic matching and simulated latency.
  check   - Re-request staging endpoints and detect breaking API contract changes using semantic JSON diff.
  version - Display version, commit, and build information.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logger.Init(logFormat, verbose, cmd.ErrOrStderr())
	},
}

func init() {
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose debug logging")
	RootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log output format: text or json")
}

// Execute runs the root Cobra CLI command.
func Execute() error {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
