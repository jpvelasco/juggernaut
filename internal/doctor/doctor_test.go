package doctor_test

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v4/internal/doctor"
)

func TestReport_AllOK(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Juggernaut block", doctor.OK, "found (schemaVersion 2)")
	r.Check("Auth mode", doctor.OK, "iam")
	r.Check("Region", doctor.OK, "us-west-2")

	if r.HasFailures() {
		t.Error("expected no failures")
	}
	if r.HasWarnings() {
		t.Error("expected no warnings")
	}
}

func TestReport_Failures(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Juggernaut block", doctor.Fail, "not found in settings.json")

	if !r.HasFailures() {
		t.Error("expected failures")
	}
	out := r.String()
	if !strings.Contains(out, "FAIL") {
		t.Error("expected FAIL in output")
	}
}

func TestReport_Warnings(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Region", doctor.Warn, "us-fake-1 is not a known Bedrock region")

	if r.HasFailures() {
		t.Error("expected no failures")
	}
	if !r.HasWarnings() {
		t.Error("expected warnings")
	}
}

func TestReport_JSON(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Region", doctor.Warn, "us-fake-1 is not a known Bedrock region")

	j, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if !strings.Contains(string(j), "WARN") {
		t.Error("expected WARN in JSON output")
	}
	if !strings.Contains(string(j), "Region") {
		t.Error("expected Region label in JSON output")
	}
}
