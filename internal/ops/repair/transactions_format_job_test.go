package repair

import (
	"testing"
	"time"
)

func TestBuildTransactionFormatPatchPlannedClearsStateAndReservedRefs(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	legacyID := "legacy_ref"

	row := transactionsFormatRow{
		ID:                 "11111111-1111-1111-1111-111111111111",
		Status:             "planned",
		PostedAt:           &now,
		CancelledAt:        &now,
		BudgetMemberID:     &legacyID,
		IncomeSourceID:     &legacyID,
		DebtID:             &legacyID,
		GoalID:             &legacyID,
		InvestmentID:       &legacyID,
		RecurringPaymentID: &legacyID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	patch := buildTransactionFormatPatch(row)
	if !patch.NeedsFix {
		t.Fatal("expected patch to require fix")
	}
	if !patch.HasTimestamp {
		t.Fatal("expected timestamp fix")
	}
	if !patch.HasReservedIDs {
		t.Fatal("expected reserved refs fix")
	}
	if patch.PostedAt != nil {
		t.Fatalf("expected posted_at to be cleared, got %v", patch.PostedAt)
	}
	if patch.CancelledAt != nil {
		t.Fatalf("expected cancelled_at to be cleared, got %v", patch.CancelledAt)
	}
}

func TestBuildTransactionFormatPatchPostedBackfillsPostedAt(t *testing.T) {
	occurred := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)

	row := transactionsFormatRow{
		ID:         "22222222-2222-2222-2222-222222222222",
		Status:     "posted",
		OccurredAt: &occurred,
		UpdatedAt:  updated,
		CreatedAt:  occurred,
	}

	patch := buildTransactionFormatPatch(row)
	if !patch.NeedsFix {
		t.Fatal("expected patch to require fix")
	}
	if !patch.HasTimestamp {
		t.Fatal("expected timestamp fix")
	}
	if patch.PostedAt == nil {
		t.Fatal("expected posted_at to be backfilled")
	}
	if !patch.PostedAt.Equal(occurred) {
		t.Fatalf("expected posted_at=%s, got %s", occurred.Format(time.RFC3339), patch.PostedAt.Format(time.RFC3339))
	}
}

func TestBuildTransactionFormatPatchCancelledBackfillsCancelledAt(t *testing.T) {
	updated := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	created := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	row := transactionsFormatRow{
		ID:        "33333333-3333-3333-3333-333333333333",
		Status:    "cancelled",
		UpdatedAt: updated,
		CreatedAt: created,
	}

	patch := buildTransactionFormatPatch(row)
	if !patch.NeedsFix {
		t.Fatal("expected patch to require fix")
	}
	if patch.CancelledAt == nil {
		t.Fatal("expected cancelled_at to be backfilled")
	}
	if !patch.CancelledAt.Equal(updated) {
		t.Fatalf("expected cancelled_at=%s, got %s", updated.Format(time.RFC3339), patch.CancelledAt.Format(time.RFC3339))
	}
}

func TestBuildTransactionFormatPatchNoopWhenAlreadyNormalized(t *testing.T) {
	posted := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	row := transactionsFormatRow{
		ID:         "44444444-4444-4444-4444-444444444444",
		Status:     "posted",
		PostedAt:   &posted,
		OccurredAt: &posted,
		CreatedAt:  posted,
		UpdatedAt:  posted,
	}

	patch := buildTransactionFormatPatch(row)
	if patch.NeedsFix {
		t.Fatalf("expected no fixes, got patch=%+v", patch)
	}
}
