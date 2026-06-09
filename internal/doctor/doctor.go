// Package doctor provides a Report type for structured diagnostic output.
package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Status represents the result of a single diagnostic check.
type Status string

// OK, Warn, and Fail are the possible check statuses.
const (
	OK   Status = "OK"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

// Entry holds the result of a single diagnostic check.
type Entry struct {
	Label  string `json:"label"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Report accumulates diagnostic check results.
type Report struct {
	entries []Entry
}

// NewReport creates an empty Report.
func NewReport() *Report {
	return &Report{}
}

// Check records a single diagnostic result.
func (r *Report) Check(label string, status Status, detail string) {
	r.entries = append(r.entries, Entry{Label: label, Status: status, Detail: detail})
}

// HasFailures returns true if any check recorded a Fail status.
func (r *Report) HasFailures() bool {
	for _, e := range r.entries {
		if e.Status == Fail {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any check recorded a Warn status.
func (r *Report) HasWarnings() bool {
	for _, e := range r.entries {
		if e.Status == Warn {
			return true
		}
	}
	return false
}

// String returns a human-readable summary of all checks.
func (r *Report) String() string {
	var sb strings.Builder
	for _, e := range r.entries {
		fmt.Fprintf(&sb, "  %-8s %-30s %s\n", "["+string(e.Status)+"]", e.Label, e.Detail)
	}
	return sb.String()
}

// JSON returns the check results as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r.entries, "", "  ")
}
