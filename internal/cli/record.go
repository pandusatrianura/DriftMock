package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/pandusatrianura/driftmock/internal/logger"
	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/proxy"
	"github.com/pandusatrianura/driftmock/internal/sanitizer"
	"github.com/pandusatrianura/driftmock/internal/storage"
	"github.com/spf13/cobra"
)

// RecordOptions holds configuration parameters for the record command.
type RecordOptions struct {
	Target string
	Port   int
	OutDir string
}

var recordOpts RecordOptions

// NewRecordCmd creates the Cobra command for record mode.
func NewRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record live HTTP traffic to an upstream target and generate sanitized JSON stubs",
		Long: `Intercepts HTTP traffic sent to the local proxy port, forwards requests to the live target,
redacts sensitive authentication headers and JSON keys, and saves stubs to disk.`,
		Example: `  # Record traffic forwarded to staging API
  driftmock record --target https://api.staging.example.com --port 8080 --out ./.drift

  # Short flags
  driftmock record -t https://api.staging.example.com -p 8080

  # Via environment variables
  DRIFTMOCK_TARGET=https://api.staging.example.com driftmock record`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			applyRecordEnvVars(cmd, &recordOpts)
			if strings.TrimSpace(recordOpts.Target) == "" {
				return fmt.Errorf("target URL is required (use --target or -t, or set DRIFTMOCK_TARGET)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return RunRecord(ctx, recordOpts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&recordOpts.Target, "target", "t", "", "target staging/development backend base URL (required or set DRIFTMOCK_TARGET)")
	cmd.Flags().IntVarP(&recordOpts.Port, "port", "p", 8080, "local port for the reverse proxy to listen on (or set DRIFTMOCK_PORT)")
	cmd.Flags().StringVarP(&recordOpts.OutDir, "out", "o", "./.drift", "directory where recorded stubs will be stored (or set DRIFTMOCK_STUBS_DIR)")

	return cmd
}

func init() {
	RootCmd.AddCommand(NewRecordCmd())
}

// ValidateRecordOptions checks whether target URL and port parameters are valid.
func ValidateRecordOptions(opts *RecordOptions) (*url.URL, error) {
	if strings.TrimSpace(opts.Target) == "" {
		return nil, fmt.Errorf("target URL is required (use --target or -t, or set DRIFTMOCK_TARGET)")
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

	if opts.Port <= 0 || opts.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d: port must be between 1 and 65535", opts.Port)
	}

	if strings.TrimSpace(opts.OutDir) == "" {
		opts.OutDir = "./.drift"
	}

	return parsed, nil
}

// RunRecord starts the recorder reverse proxy server and blocks until ctx is canceled.
func RunRecord(ctx context.Context, opts RecordOptions, out io.Writer) error {
	targetURL, err := ValidateRecordOptions(&opts)
	if err != nil {
		return err
	}

	store, err := storage.NewFileStore(opts.OutDir)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	pipeline := sanitizer.NewPipeline()

	recorder, err := proxy.NewRecorderProxy(targetURL, store, pipeline)
	if err != nil {
		return fmt.Errorf("failed to initialize recorder proxy: %w", err)
	}

	// Colored terminal logging
	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed, color.Bold).SprintFunc()

	recorder.OnStubSaved = func(stub *model.Stub, filePath string) {
		if logger.IsJSON() {
			slog.Info("recorded stub",
				"method", stub.Request.Method,
				"path", stub.Request.Path,
				"status", stub.Response.StatusCode,
				"file", filepath.Base(filePath),
				"latency_ms", stub.Response.LatencyMs,
			)
			return
		}

		statusStr := fmt.Sprintf("%d", stub.Response.StatusCode)
		if stub.Response.StatusCode >= 200 && stub.Response.StatusCode < 300 {
			statusStr = green(statusStr)
		} else if stub.Response.StatusCode >= 400 {
			statusStr = red(statusStr)
		} else {
			statusStr = yellow(statusStr)
		}

		fmt.Fprintf(out, "[%s] %-6s %s %s -> %s (%dms)\n",
			green("RECORDED"),
			cyan(stub.Request.Method),
			stub.Request.Path,
			statusStr,
			filepath.Base(filePath),
			stub.Response.LatencyMs,
		)
	}

	recorder.OnError = func(err error) {
		if logger.IsJSON() {
			slog.Error("proxy error", "error", err)
			return
		}
		fmt.Fprintf(out, "[%s] %v\n", red("PROXY ERROR"), err)
	}

	serverAddr := fmt.Sprintf(":%d", opts.Port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: recorder,
	}

	if logger.IsJSON() {
		slog.Info("starting driftmock recorder",
			"target", targetURL.String(),
			"port", opts.Port,
			"out", store.StubsDir(),
		)
	} else {
		// Print startup banner
		fmt.Fprintf(out, "\n%s\n", cyan("╔══════════════════════════════════════════════════════════════╗"))
		fmt.Fprintf(out, "%s\n", cyan("║             DriftMock Traffic Recorder (Active)              ║"))
		fmt.Fprintf(out, "%s\n\n", cyan("╚══════════════════════════════════════════════════════════════╝"))
		fmt.Fprintf(out, "  %s  %s\n", yellow("Target Upstream:"), targetURL.String())
		fmt.Fprintf(out, "  %s  http://localhost:%d\n", yellow("Proxy Listening:"), opts.Port)
		fmt.Fprintf(out, "  %s  %s\n\n", yellow("Stubs Output:   "), store.StubsDir())
		fmt.Fprintf(out, "Point your frontend API client to %s to begin recording.\n", green(fmt.Sprintf("http://localhost:%d", opts.Port)))
		fmt.Fprintf(out, "Press %s to stop recording and exit.\n\n", yellow("Ctrl+C"))
	}

	serverErrChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrChan <- err
		}
	}()

	select {
	case err := <-serverErrChan:
		return fmt.Errorf("proxy server error: %w", err)
	case <-ctx.Done():
		server.SetKeepAlivesEnabled(false)
		if logger.IsJSON() {
			slog.Info("shutting down recording proxy gracefully")
		} else {
			fmt.Fprintf(out, "\n%s Shutting down recording proxy gracefully...\n", yellow("[STOPPING]"))
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during server shutdown: %w", err)
		}
		if logger.IsJSON() {
			slog.Info("proxy stopped", "stubs_dir", store.StubsDir())
		} else {
			fmt.Fprintf(out, "%s Proxy stopped. Recorded stubs are saved in %s\n", green("[DONE]"), store.StubsDir())
		}
		return nil
	}
}
