package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/pandusatrianura/driftmock/internal/matcher"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/proxy"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/spf13/cobra"
)

// ReplayCommandOptions holds configuration parameters for the replay command.
type ReplayCommandOptions struct {
	StubsDir      string
	Port          int
	SimulateDelay bool
	LatencyMs     int
}

var replayOpts ReplayCommandOptions

// NewReplayCmd creates the Cobra command for replay mode.
func NewReplayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Serve recorded JSON mock stubs offline with deterministic matching",
		Long: `Starts a local offline mock HTTP server that deterministically serves recorded stubs.
Supports exact matching, dynamic parameter route fallback (e.g. /users/:id),
permissive CORS headers for web browsers, and optional latency simulation.`,
		Example: `  # Start mock server on port 8080 from default ./.drift
  driftmock replay

  # Custom port and stubs directory
  driftmock replay --stubs ./mocks --port 3000

  # Via environment variables
  DRIFTMOCK_PORT=3000 DRIFTMOCK_STUBS_DIR=./mocks driftmock replay

  # Simulate original recorded network latency
  driftmock replay --simulate-delay

  # Enforce fixed simulated latency of 250ms
  driftmock replay --latency 250`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			applyReplayEnvVars(cmd, &replayOpts)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return RunReplay(ctx, replayOpts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&replayOpts.StubsDir, "stubs", "s", "./.drift", "directory where recorded stubs are stored (or set DRIFTMOCK_STUBS_DIR)")
	cmd.Flags().IntVarP(&replayOpts.Port, "port", "p", 8080, "local port for the mock server to listen on (or set DRIFTMOCK_PORT)")
	cmd.Flags().BoolVar(&replayOpts.SimulateDelay, "simulate-delay", false, "simulate recorded latency from each stub (or set DRIFTMOCK_SIMULATE_DELAY)")
	cmd.Flags().IntVar(&replayOpts.LatencyMs, "latency", 0, "fixed latency simulation in milliseconds (or set DRIFTMOCK_LATENCY_MS)")

	return cmd
}

func init() {
	RootCmd.AddCommand(NewReplayCmd())
}

// ValidateReplayOptions validates port and latency parameters.
func ValidateReplayOptions(opts *ReplayCommandOptions) error {
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("invalid port %d: port must be between 1 and 65535", opts.Port)
	}

	if opts.LatencyMs < 0 {
		return fmt.Errorf("invalid latency %dms: latency cannot be negative", opts.LatencyMs)
	}

	if strings.TrimSpace(opts.StubsDir) == "" {
		opts.StubsDir = "./.drift"
	}

	return nil
}

// RunReplay starts the offline mock server and blocks until ctx is canceled.
func RunReplay(ctx context.Context, opts ReplayCommandOptions, out io.Writer) error {
	if err := ValidateReplayOptions(&opts); err != nil {
		return err
	}

	store, err := storage.NewFileStore(opts.StubsDir)
	if err != nil {
		return fmt.Errorf("failed to open stubs storage: %w", err)
	}

	var fixedLatency time.Duration
	if opts.LatencyMs > 0 {
		fixedLatency = time.Duration(opts.LatencyMs) * time.Millisecond
	}

	replayer, err := proxy.NewReplayer(store, proxy.ReplayOptions{
		SimulateDelay: opts.SimulateDelay,
		FixedLatency:  fixedLatency,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize replayer: %w", err)
	}

	// Colored terminal formatting
	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed, color.Bold).SprintFunc()
	magenta := color.New(color.FgMagenta).SprintFunc()

	replayer.OnMatch = func(stub *model.Stub, tier matcher.MatchTier, r *http.Request) {
		if logger.IsJSON() {
			slog.Info("replayed mock",
				"method", r.Method,
				"path", r.URL.Path,
				"status", stub.Response.StatusCode,
				"tier", string(tier),
				"stub_id", stub.ID,
			)
			return
		}

		tierBadge := string(tier)
		if tier == matcher.MatchTierExact {
			tierBadge = green(tierBadge)
		} else if tier == matcher.MatchTierPattern {
			tierBadge = magenta(tierBadge)
		} else {
			tierBadge = yellow(tierBadge)
		}

		fmt.Fprintf(out, "[%s] %-6s %-30s %d (%s) -> %s\n",
			green("HIT"),
			cyan(r.Method),
			r.URL.Path,
			stub.Response.StatusCode,
			tierBadge,
			stub.ID,
		)
	}

	replayer.OnMiss = func(r *http.Request) {
		if logger.IsJSON() {
			slog.Warn("mock not found",
				"method", r.Method,
				"path", r.URL.Path,
			)
			return
		}

		fmt.Fprintf(out, "[%s] %-6s %-30s 404 (No matching stub)\n",
			red("MISS"),
			cyan(r.Method),
			r.URL.Path,
		)
	}

	serverAddr := fmt.Sprintf(":%d", opts.Port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: replayer,
	}

	// Determine latency description
	latencyDesc := "Snappy (<5ms default)"
	if opts.LatencyMs > 0 {
		latencyDesc = fmt.Sprintf("Fixed (%dms)", opts.LatencyMs)
	} else if opts.SimulateDelay {
		latencyDesc = "Simulated (from recorded stubs)"
	}

	routes := replayer.Matcher().RegisteredRoutes()

	if logger.IsJSON() {
		slog.Info("starting driftmock replayer",
			"port", opts.Port,
			"stubs_dir", store.StubsDir(),
			"stubs_count", len(routes),
			"latency_mode", latencyDesc,
		)
	} else {
		// Print startup banner
		fmt.Fprintf(out, "\n%s\n", cyan("╔══════════════════════════════════════════════════════════════╗"))
		fmt.Fprintf(out, "%s\n", cyan("║              DriftMock Offline Replay (Active)               ║"))
		fmt.Fprintf(out, "%s\n\n", cyan("╚══════════════════════════════════════════════════════════════╝"))
		fmt.Fprintf(out, "  %s  http://localhost:%d\n", yellow("Mock Server:   "), opts.Port)
		fmt.Fprintf(out, "  %s  %s (%d stubs loaded)\n", yellow("Stubs Source:  "), store.StubsDir(), len(routes))
		fmt.Fprintf(out, "  %s  %s\n", yellow("Latency Mode:  "), latencyDesc)
		fmt.Fprintf(out, "  %s  Permissive (All origins allowed)\n\n", yellow("CORS Handling: "))

		if len(routes) > 0 {
			fmt.Fprintf(out, "%s\n", yellow("Registered Routes:"))
			for _, route := range routes {
				fmt.Fprintf(out, "  • %s\n", route)
			}
			fmt.Fprintln(out)
		} else {
			fmt.Fprintf(out, "%s No stubs found in '%s'. Run 'driftmock record' first or add .json stubs.\n\n",
				yellow("Notice:"), store.StubsDir())
		}

		fmt.Fprintf(out, "Point your frontend API client to %s to use offline mock.\n", green(fmt.Sprintf("http://localhost:%d", opts.Port)))
		fmt.Fprintf(out, "Press %s to stop mock server and exit.\n\n", yellow("Ctrl+C"))
	}

	serverErrChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrChan <- err
		}
	}()

	select {
	case err := <-serverErrChan:
		return fmt.Errorf("mock server error: %w", err)
	case <-ctx.Done():
		server.SetKeepAlivesEnabled(false)
		if logger.IsJSON() {
			slog.Info("shutting down mock server gracefully")
		} else {
			fmt.Fprintf(out, "\n%s Shutting down mock server gracefully...\n", yellow("[STOPPING]"))
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during server shutdown: %w", err)
		}
		if logger.IsJSON() {
			slog.Info("mock server stopped")
		} else {
			fmt.Fprintf(out, "%s Mock server stopped.\n", green("[DONE]"))
		}
		return nil
	}
}
