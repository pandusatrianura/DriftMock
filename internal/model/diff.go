package model

// DiffSeverity represents the severity level of detected contract drift.
type DiffSeverity string

const (
	// SeverityBreaking indicates a backward-incompatible change (field removed, type mismatch).
	SeverityBreaking DiffSeverity = "BREAKING"
	// SeverityWarning indicates a potential issue or value drift (e.g., unexpected enum value).
	SeverityWarning DiffSeverity = "WARNING"
	// SeverityAddition indicates a backward-compatible addition (new optional/nullable field).
	SeverityAddition DiffSeverity = "NON_BREAKING"
)

// DiffItem describes a single difference found between expected baseline and received response.
type DiffItem struct {
	Field    string       `json:"field"`
	Severity DiffSeverity `json:"severity"`
	Message  string       `json:"message"`
	Expected any          `json:"expected,omitempty"`
	Received any          `json:"received,omitempty"`
}

// DiffReport groups all diff items for a specific endpoint interaction.
type DiffReport struct {
	Method string     `json:"method"`
	Path   string     `json:"path"`
	StubID string     `json:"stub_id"`
	Items  []DiffItem `json:"items"`
}

// HasBreaking returns true if any diff item in the report is classified as breaking.
func (r *DiffReport) HasBreaking() bool {
	for _, item := range r.Items {
		if item.Severity == SeverityBreaking {
			return true
		}
	}
	return false
}

// BreakingCount returns the count of breaking changes in the report.
func (r *DiffReport) BreakingCount() int {
	count := 0
	for _, item := range r.Items {
		if item.Severity == SeverityBreaking {
			count++
		}
	}
	return count
}

// WarningCount returns the count of warnings in the report.
func (r *DiffReport) WarningCount() int {
	count := 0
	for _, item := range r.Items {
		if item.Severity == SeverityWarning {
			count++
		}
	}
	return count
}

// AdditionCount returns the count of non-breaking additions in the report.
func (r *DiffReport) AdditionCount() int {
	count := 0
	for _, item := range r.Items {
		if item.Severity == SeverityAddition {
			count++
		}
	}
	return count
}
