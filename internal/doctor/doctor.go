package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Status string

const (
	OK   Status = "OK"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

type Entry struct {
	Label  string `json:"label"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	entries []Entry
}

func NewReport() *Report {
	return &Report{}
}

func (r *Report) Check(label string, status Status, detail string) {
	r.entries = append(r.entries, Entry{Label: label, Status: status, Detail: detail})
}

func (r *Report) HasFailures() bool {
	for _, e := range r.entries {
		if e.Status == Fail {
			return true
		}
	}
	return false
}

func (r *Report) HasWarnings() bool {
	for _, e := range r.entries {
		if e.Status == Warn {
			return true
		}
	}
	return false
}

func (r *Report) String() string {
	var sb strings.Builder
	for _, e := range r.entries {
		fmt.Fprintf(&sb, "  %-8s %-30s %s\n", "["+string(e.Status)+"]", e.Label, e.Detail)
	}
	return sb.String()
}

func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r.entries, "", "  ")
}
