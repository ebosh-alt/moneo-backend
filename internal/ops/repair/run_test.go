package repair

import "testing"

func TestParseTransactionsFormatOptionsDefaults(t *testing.T) {
	opts, err := parseTransactionsFormatOptions(nil)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}

	if opts.DryRun {
		t.Fatal("expected dry-run false by default")
	}
	if opts.BatchSize != defaultBatchSize {
		t.Fatalf("expected default batch-size %d, got %d", defaultBatchSize, opts.BatchSize)
	}
	if opts.Limit != 0 {
		t.Fatalf("expected default limit 0, got %d", opts.Limit)
	}
}

func TestParseTransactionsFormatOptionsCustom(t *testing.T) {
	opts, err := parseTransactionsFormatOptions([]string{"--dry-run", "--batch-size=250", "--limit=1000"})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}

	if !opts.DryRun {
		t.Fatal("expected dry-run true")
	}
	if opts.BatchSize != 250 {
		t.Fatalf("expected batch-size 250, got %d", opts.BatchSize)
	}
	if opts.Limit != 1000 {
		t.Fatalf("expected limit 1000, got %d", opts.Limit)
	}
}

func TestParseTransactionsFormatOptionsValidation(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "negative limit", args: []string{"--limit=-1"}},
		{name: "zero batch-size", args: []string{"--batch-size=0"}},
		{name: "too large batch-size", args: []string{"--batch-size=6000"}},
		{name: "extra args", args: []string{"--dry-run", "unexpected"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseTransactionsFormatOptions(tc.args); err == nil {
				t.Fatalf("expected parse error for args %v", tc.args)
			}
		})
	}
}

func TestParseTransactionsVerifyOptionsDefaults(t *testing.T) {
	opts, err := parseTransactionsVerifyOptions(nil)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}

	if opts.BatchSize != defaultBatchSize {
		t.Fatalf("expected default batch-size %d, got %d", defaultBatchSize, opts.BatchSize)
	}
	if opts.Limit != 0 {
		t.Fatalf("expected default limit 0, got %d", opts.Limit)
	}
	if opts.SampleLimit != 50 {
		t.Fatalf("expected default sample-limit 50, got %d", opts.SampleLimit)
	}
}

func TestParseTransactionsVerifyOptionsCustom(t *testing.T) {
	opts, err := parseTransactionsVerifyOptions([]string{
		"--batch-size=250",
		"--limit=1000",
		"--sample-limit=30",
		"--baseline-out=/tmp/before.json",
		"--baseline-in=/tmp/after.json",
		"--report-file=/tmp/report.md",
		"--max-updated-at=2026-05-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}

	if opts.BatchSize != 250 {
		t.Fatalf("expected batch-size 250, got %d", opts.BatchSize)
	}
	if opts.Limit != 1000 {
		t.Fatalf("expected limit 1000, got %d", opts.Limit)
	}
	if opts.SampleLimit != 30 {
		t.Fatalf("expected sample-limit 30, got %d", opts.SampleLimit)
	}
	if opts.BaselineOut != "/tmp/before.json" {
		t.Fatalf("unexpected baseline-out %q", opts.BaselineOut)
	}
	if opts.BaselineIn != "/tmp/after.json" {
		t.Fatalf("unexpected baseline-in %q", opts.BaselineIn)
	}
	if opts.ReportFile != "/tmp/report.md" {
		t.Fatalf("unexpected report-file %q", opts.ReportFile)
	}
	if opts.MaxUpdatedAtRFC3339 != "2026-05-01T00:00:00Z" {
		t.Fatalf("unexpected max-updated-at %q", opts.MaxUpdatedAtRFC3339)
	}
}

func TestParseTransactionsVerifyOptionsValidation(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "invalid batch-size", args: []string{"--batch-size=0"}},
		{name: "too large batch-size", args: []string{"--batch-size=7000"}},
		{name: "negative limit", args: []string{"--limit=-1"}},
		{name: "invalid sample-limit", args: []string{"--sample-limit=0"}},
		{name: "extra args", args: []string{"--limit=10", "unexpected"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseTransactionsVerifyOptions(tc.args); err == nil {
				t.Fatalf("expected parse error for args %v", tc.args)
			}
		})
	}
}
