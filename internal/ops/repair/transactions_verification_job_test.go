package repair

import (
	"strings"
	"testing"
	"time"
)

func TestDiscrepanciesFromCurrentSnapshot(t *testing.T) {
	snapshot := TransactionsVerificationSnapshot{
		Invariants: TransactionsVerificationInvariantCounts{
			PostedWithoutPostedAt:       2,
			CancelledWithoutCancelledAt: 1,
			PlannedWithPostedAt:         3,
			ReservedRefsPresent:         4,
		},
	}

	discrepancies := discrepanciesFromCurrentSnapshot(snapshot)
	if len(discrepancies) == 0 {
		t.Fatal("expected discrepancies")
	}

	assertHasDiscrepancy(t, discrepancies, "posted_without_posted_at", SeverityHigh)
	assertHasDiscrepancy(t, discrepancies, "cancelled_without_cancelled_at", SeverityHigh)
	assertHasDiscrepancy(t, discrepancies, "planned_with_posted_at", SeverityMedium)
	assertHasDiscrepancy(t, discrepancies, "reserved_refs_present", SeverityMedium)
}

func TestCompareSnapshotsDetectsRowsNeedingRepairDelta(t *testing.T) {
	before := TransactionsVerificationSnapshot{
		Totals: TransactionsVerificationTotals{Scanned: 100},
		Aggregates: TransactionsVerificationAggregates{
			RowsNeedingRepair: 10,
		},
	}
	after := TransactionsVerificationSnapshot{
		Totals: TransactionsVerificationTotals{Scanned: 100},
		Aggregates: TransactionsVerificationAggregates{
			RowsNeedingRepair: 3,
		},
	}

	discrepancies := compareSnapshots(before, after)
	assertHasDiscrepancy(t, discrepancies, "delta_rows_needing_repair", SeverityHigh)
}

func TestRenderVerificationReportMarkdownContainsSummary(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	report := TransactionsVerificationReport{
		GeneratedAt:      now,
		BaselineCompared: true,
		BaselinePath:     "/tmp/before.json",
		Snapshot: TransactionsVerificationSnapshot{
			Scope: TransactionsVerificationScope{
				Limit: 100,
			},
			Totals: TransactionsVerificationTotals{
				Scanned: 42,
			},
		},
		Discrepancies: []TransactionsVerificationDiscrepancy{
			{
				Code:        "posted_without_posted_at",
				Severity:    SeverityHigh,
				Description: "Posted transactions without posted_at",
				After:       "2",
				Remediation: "Run repair job",
			},
		},
	}

	markdown := renderVerificationReportMarkdown(report)
	contains := []string{
		"# Migration Verification Report",
		"`/tmp/before.json`",
		"`42`",
		"`posted_without_posted_at`",
		"`HIGH`",
	}
	for _, needle := range contains {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("expected markdown to contain %q", needle)
		}
	}
}

func assertHasDiscrepancy(
	t *testing.T,
	discrepancies []TransactionsVerificationDiscrepancy,
	code string,
	severity VerificationSeverity,
) {
	t.Helper()
	for _, item := range discrepancies {
		if item.Code == code && item.Severity == severity {
			return
		}
	}
	t.Fatalf("expected discrepancy code=%s severity=%s in %+v", code, severity, discrepancies)
}
