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
