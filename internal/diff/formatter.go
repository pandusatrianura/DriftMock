package diff

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/pandusatrianura/driftmock/internal/model"
)

// Formatter handles rendering diff reports into high-contrast, Jest/Vitest-styled terminal output.
type Formatter struct {
	red     func(a ...any) string
	green   func(a ...any) string
	yellow  func(a ...any) string
	cyan    func(a ...any) string
	bold    func(a ...any) string
	boldRed func(a ...any) string
	boldGrn func(a ...any) string
}

// NewFormatter creates an initialized terminal formatter.
func NewFormatter() *Formatter {
	return &Formatter{
		red:     color.New(color.FgRed).SprintFunc(),
		green:   color.New(color.FgGreen).SprintFunc(),
		yellow:  color.New(color.FgYellow).SprintFunc(),
		cyan:    color.New(color.FgCyan).SprintFunc(),
		bold:    color.New(color.Bold).SprintFunc(),
		boldRed: color.New(color.FgRed, color.Bold).SprintFunc(),
		boldGrn: color.New(color.FgGreen, color.Bold).SprintFunc(),
	}
}

// FormatReport writes a formatted diff report for a single endpoint to w.
func (f *Formatter) FormatReport(w io.Writer, report *model.DiffReport) {
	if report == nil {
		return
	}

	endpointHeader := fmt.Sprintf("%s %s", report.Method, report.Path)

	if len(report.Items) == 0 {
		fmt.Fprintf(w, "[%s] %s %s\n", f.cyan("DriftMock"), f.boldGrn("✅ CONTRACT VERIFIED:"), endpointHeader)
		fmt.Fprintf(w, "  %s\n\n", f.green("No contract changes detected. 100% compatible."))
		return
	}

	if report.HasBreaking() {
		fmt.Fprintf(w, "[%s] %s %s\n", f.cyan("DriftMock"), f.boldRed("⚠️ CONTRACT DRIFT DETECTED:"), f.bold(endpointHeader))
	} else {
		fmt.Fprintf(w, "[%s] %s %s\n", f.cyan("DriftMock"), f.yellow("ℹ️ CONTRACT MODIFIED (NON-BREAKING):"), f.bold(endpointHeader))
	}

	fmt.Fprintln(w)

	for _, item := range report.Items {
		switch item.Severity {
		case model.SeverityBreaking:
			prefix := f.boldRed("-")
			if strings.Contains(strings.ToLower(item.Message), "mismatch") || strings.Contains(strings.ToLower(item.Message), "null") {
				prefix = f.boldRed("~")
			}
			fmt.Fprintf(w, "  %s %s: %s\n", prefix, f.bold(item.Field), f.red(item.Message))

		case model.SeverityWarning:
			prefix := f.yellow("~")
			fmt.Fprintf(w, "  %s %s: %s\n", prefix, f.bold(item.Field), f.yellow(item.Message))

		case model.SeverityAddition:
			prefix := f.green("+")
			fmt.Fprintf(w, "  %s %s: %s\n", prefix, f.bold(item.Field), f.green(item.Message))
		}
	}

	fmt.Fprintln(w)

	breakingCount := report.BreakingCount()
	warningCount := report.WarningCount()
	additionCount := report.AdditionCount()

	if breakingCount > 0 {
		fmt.Fprintf(w, "  Status: %s %d breaking changes found compared to stub '%s'\n\n",
			f.boldRed("❌"), breakingCount, report.StubID)
	} else {
		fmt.Fprintf(w, "  Status: %s Backward-compatible (%d additions, %d warnings) compared to stub '%s'\n\n",
			f.boldGrn("✅"), additionCount, warningCount, report.StubID)
	}
}

// SummaryTotals aggregates metrics across multiple diff reports.
type SummaryTotals struct {
	TotalChecked   int
	PassedCount    int
	FailedCount    int
	TotalBreaking  int
	TotalWarnings  int
	TotalAdditions int
}

// FormatSummary writes an aggregate summary box of all checked endpoints to w.
func (f *Formatter) FormatSummary(w io.Writer, reports []*model.DiffReport) SummaryTotals {
	totals := SummaryTotals{
		TotalChecked: len(reports),
	}

	for _, r := range reports {
		breaking := r.BreakingCount()
		totals.TotalBreaking += breaking
		totals.TotalWarnings += r.WarningCount()
		totals.TotalAdditions += r.AdditionCount()

		if breaking > 0 {
			totals.FailedCount++
		} else {
			totals.PassedCount++
		}
	}

	divider := strings.Repeat("═", 64)
	fmt.Fprintf(w, "\n%s\n", f.cyan(divider))
	fmt.Fprintf(w, "%s\n", f.bold("                     Contract Drift Summary"))
	fmt.Fprintf(w, "%s\n\n", f.cyan(divider))

	fmt.Fprintf(w, "  Endpoints Checked: %s\n", f.bold(fmt.Sprintf("%d", totals.TotalChecked)))
	fmt.Fprintf(w, "  Passed:            %s\n", f.green(fmt.Sprintf("%d", totals.PassedCount)))
	if totals.FailedCount > 0 {
		fmt.Fprintf(w, "  Failed (Breaking): %s\n\n", f.boldRed(fmt.Sprintf("%d", totals.FailedCount)))
	} else {
		fmt.Fprintf(w, "  Failed (Breaking): %s\n\n", f.green("0"))
	}

	fmt.Fprintln(w, "  Total Differences:")
	if totals.TotalBreaking > 0 {
		fmt.Fprintf(w, "    %s %d Breaking changes\n", f.boldRed("🔴"), totals.TotalBreaking)
	} else {
		fmt.Fprintf(w, "    %s 0 Breaking changes\n", f.green("🔴"))
	}
	fmt.Fprintf(w, "    %s %d Warnings\n", f.yellow("🟡"), totals.TotalWarnings)
	fmt.Fprintf(w, "    %s %d Additions (Non-breaking)\n\n", f.green("🟢"), totals.TotalAdditions)

	if totals.FailedCount > 0 {
		fmt.Fprintf(w, "  Final Verdict: %s\n", f.boldRed("❌ FAILED (breaking contract drift detected)"))
	} else {
		fmt.Fprintf(w, "  Final Verdict: %s\n", f.boldGrn("✅ PASSED (all endpoints backward-compatible)"))
	}
	fmt.Fprintf(w, "%s\n\n", f.cyan(divider))

	return totals
}
