package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pandusatrianura/driftmock/internal/diff"
	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/spf13/cobra"
)

// CheckOptions holds configuration parameters for the check command.
type CheckOptions struct {
	Target         string
	StubsDir       string
	Strict         bool
	IgnoreVolatile bool
	TimeoutSeconds int
}

var checkOpts CheckOptions

// NewCheckCmd creates the Cobra command for check mode.
func NewCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Detect API contract drift between recorded stubs and a live target server",
		Long: `Re-executes all recorded interactions against the specified live target environment
and runs semantic JSON AST tree diffs to detect breaking changes, type mismatches,
removed fields, and backward-compatible additions.`,
		Example: `  # Run drift check against staging environment
  driftmock check --target https://api.staging.example.com

  # Run drift check against custom stubs directory
  driftmock check -t https://api.staging.example.com -s ./mocks

  # Non-strict mode (exits 0 even if breaking changes exist)
  driftmock check -t https://api.staging.example.com --strict=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			hasBreaking, err := RunCheck(ctx, checkOpts, cmd.OutOrStdout())
			if err != nil {
				return err
			}

			if hasBreaking && checkOpts.Strict {
				return fmt.Errorf("contract drift detected: breaking changes found")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&checkOpts.Target, "target", "t", "", "target live staging/development backend base URL (required)")
	cmd.Flags().StringVarP(&checkOpts.StubsDir, "stubs", "s", "./.drift", "directory where recorded stubs are stored")
	cmd.Flags().BoolVar(&checkOpts.Strict, "strict", true, "fail build with exit code 1 if breaking changes are detected")
	cmd.Flags().BoolVar(&checkOpts.IgnoreVolatile, "ignore-volatile", true, "ignore volatile timestamp fields (updated_at, timestamp)")
	cmd.Flags().IntVar(&checkOpts.TimeoutSeconds, "timeout", 10, "HTTP request timeout per endpoint in seconds")

	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func init() {
	RootCmd.AddCommand(NewCheckCmd())
}

// ValidateCheckOptions verifies target URL and configuration parameters.
func ValidateCheckOptions(opts *CheckOptions) (*url.URL, error) {
	if strings.TrimSpace(opts.Target) == "" {
		return nil, fmt.Errorf("target URL is required (use --target or -t)")
	}

	parsed, err := url.Parse(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL '%s': %w", opts.Target, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid target scheme '%s': target must start with http:// or https://", parsed.Scheme)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("target URL '%s' must include a valid host", opts.Target)
	}

	if opts.TimeoutSeconds <= 0 {
		opts.TimeoutSeconds = 10
	}

	if strings.TrimSpace(opts.StubsDir) == "" {
		opts.StubsDir = "./.drift"
	}

	return parsed, nil
}

// RunCheck executes the contract drift suite against the target and formats terminal output.
// Returns hasBreaking = true if any breaking diff items are detected.
func RunCheck(ctx context.Context, opts CheckOptions, out io.Writer) (bool, error) {
	targetURL, err := ValidateCheckOptions(&opts)
	if err != nil {
		return false, err
	}

	store, err := storage.NewFileStore(opts.StubsDir)
	if err != nil {
		return false, fmt.Errorf("failed to open stubs storage: %w", err)
	}

	stubs, err := store.List()
	if err != nil {
		return false, fmt.Errorf("failed to list stubs: %w", err)
	}

	if len(stubs) == 0 {
		fmt.Fprintf(out, "No stubs found in '%s'. Run 'driftmock record' first.\n", store.StubsDir())
		return false, nil
	}

	runnerOpts := diff.RunnerOptions{
		TargetBaseURL: targetURL.String(),
		Timeout:       time.Duration(opts.TimeoutSeconds) * time.Second,
		DiffOptions: diff.DiffOptions{
			IgnoreVolatileKeys: opts.IgnoreVolatile,
			CheckEnumDrift:     true,
		},
	}

	runner := diff.NewRunner(store, runnerOpts)
	reports, err := runner.Run(ctx)
	if err != nil {
		return false, fmt.Errorf("contract check execution error: %w", err)
	}

	if logger.IsJSON() {
		for _, report := range reports {
			breaking := report.BreakingCount()
			warnings := report.WarningCount()
			additions := report.AdditionCount()
			level := slog.LevelInfo
			if breaking > 0 {
				level = slog.LevelError
			} else if warnings > 0 {
				level = slog.LevelWarn
			}
			slog.Log(ctx, level, "contract drift check result",
				"method", report.Method,
				"path", report.Path,
				"stub_id", report.StubID,
				"breaking", breaking,
				"warnings", warnings,
				"additions", additions,
			)
		}

		formatter := diff.NewFormatter()
		totals := formatter.FormatSummary(io.Discard, reports)
		slog.Info("contract drift check summary",
			"endpoints_checked", totals.TotalChecked,
			"passed", totals.PassedCount,
			"failed", totals.FailedCount,
			"breaking", totals.TotalBreaking,
			"warnings", totals.TotalWarnings,
			"additions", totals.TotalAdditions,
		)
		return totals.TotalBreaking > 0, nil
	}

	formatter := diff.NewFormatter()
	for _, report := range reports {
		formatter.FormatReport(out, report)
	}

	totals := formatter.FormatSummary(out, reports)
	hasBreaking := totals.TotalBreaking > 0

	return hasBreaking, nil
}
