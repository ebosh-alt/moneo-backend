package repair

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxIssueSamples = 100

type TransactionsFormatOptions struct {
	DryRun    bool
	BatchSize int
	Limit     int
}

type TransactionsFormatStats struct {
	Scanned       int64
	NeedsFix      int64
	Repaired      int64
	Skipped       int64
	Conflicts     int64
	Errors        int64
	ReservedFixed int64
	TimestampFix  int64
}

type TransactionsFormatJob struct {
	pool *pgxpool.Pool
	out  io.Writer
}

type transactionsFormatRow struct {
	ID                 string
	Status             string
	OccurredAt         *time.Time
	PlannedAt          *time.Time
	PostedAt           *time.Time
	CancelledAt        *time.Time
	BudgetMemberID     *string
	IncomeSourceID     *string
	DebtID             *string
	GoalID             *string
	InvestmentID       *string
	RecurringPaymentID *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type transactionFormatPatch struct {
	NeedsFix       bool
	HasTimestamp   bool
	HasReservedIDs bool

	PostedAt    *time.Time
	CancelledAt *time.Time
}

type transactionFormatIssue struct {
	ID      string
	Reason  string
	Details string
}

func NewTransactionsFormatJob(pool *pgxpool.Pool, out io.Writer) *TransactionsFormatJob {
	if out == nil {
		out = io.Discard
	}
	return &TransactionsFormatJob{
		pool: pool,
		out:  out,
	}
}

func (j *TransactionsFormatJob) Run(ctx context.Context, opts TransactionsFormatOptions) error {
	if j == nil || j.pool == nil {
		return fmt.Errorf("transactions format repair job is not initialized")
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}

	startedAt := time.Now().UTC()
	stats := TransactionsFormatStats{}
	issues := make([]transactionFormatIssue, 0, 8)

	fmt.Fprintf(
		j.out,
		"repair transactions-format started at=%s dry-run=%t batch-size=%d limit=%d\n",
		startedAt.Format(time.RFC3339),
		opts.DryRun,
		opts.BatchSize,
		opts.Limit,
	)

	var lastID string
	for {
		batchLimit := opts.BatchSize
		if opts.Limit > 0 {
			remaining := opts.Limit - int(stats.Scanned)
			if remaining <= 0 {
				break
			}
			if remaining < batchLimit {
				batchLimit = remaining
			}
		}

		rows, err := j.fetchBatch(ctx, lastID, batchLimit)
		if err != nil {
			return fmt.Errorf("fetch transactions batch: %w", err)
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			stats.Scanned++
			lastID = row.ID

			patch := buildTransactionFormatPatch(row)
			if !patch.NeedsFix {
				stats.Skipped++
				continue
			}

			stats.NeedsFix++
			if patch.HasTimestamp {
				stats.TimestampFix++
			}
			if patch.HasReservedIDs {
				stats.ReservedFixed++
			}

			if opts.DryRun {
				fmt.Fprintf(
					j.out,
					"[DRY-RUN] transaction_id=%s status=%s patch={timestamp:%t reserved_ids:%t}\n",
					row.ID,
					row.Status,
					patch.HasTimestamp,
					patch.HasReservedIDs,
				)
				continue
			}

			updated, err := j.applyPatch(ctx, row, patch, time.Now().UTC())
			if err != nil {
				stats.Errors++
				issues = appendIssue(issues, transactionFormatIssue{
					ID:      row.ID,
					Reason:  "update_failed",
					Details: err.Error(),
				})
				fmt.Fprintf(j.out, "[ERROR] transaction_id=%s update_failed=%v\n", row.ID, err)
				continue
			}
			if !updated {
				stats.Conflicts++
				issues = appendIssue(issues, transactionFormatIssue{
					ID:      row.ID,
					Reason:  "concurrent_update",
					Details: "updated_at changed during repair",
				})
				fmt.Fprintf(j.out, "[WARN] transaction_id=%s concurrent update detected, skipped\n", row.ID)
				continue
			}

			stats.Repaired++
		}

		fmt.Fprintf(
			j.out,
			"[PROGRESS] scanned=%d needs_fix=%d repaired=%d skipped=%d conflicts=%d errors=%d\n",
			stats.Scanned,
			stats.NeedsFix,
			stats.Repaired,
			stats.Skipped,
			stats.Conflicts,
			stats.Errors,
		)
	}

	finishedAt := time.Now().UTC()
	fmt.Fprintf(
		j.out,
		"repair transactions-format finished at=%s duration=%s\n",
		finishedAt.Format(time.RFC3339),
		finishedAt.Sub(startedAt).Round(time.Millisecond),
	)
	fmt.Fprintf(
		j.out,
		"report: scanned=%d needs_fix=%d repaired=%d skipped=%d conflicts=%d errors=%d timestamp_fix=%d reserved_fix=%d dry_run=%t\n",
		stats.Scanned,
		stats.NeedsFix,
		stats.Repaired,
		stats.Skipped,
		stats.Conflicts,
		stats.Errors,
		stats.TimestampFix,
		stats.ReservedFixed,
		opts.DryRun,
	)

	if len(issues) > 0 {
		fmt.Fprintf(j.out, "issue_samples(%d):\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(j.out, "  - transaction_id=%s reason=%s details=%s\n", issue.ID, issue.Reason, issue.Details)
		}
	}

	return nil
}

func (j *TransactionsFormatJob) fetchBatch(ctx context.Context, lastID string, limit int) ([]transactionsFormatRow, error) {
	query := `
SELECT
	id::text,
	status,
	occurred_at,
	planned_at,
	posted_at,
	cancelled_at,
	budget_member_id::text,
	income_source_id::text,
	debt_id::text,
	goal_id::text,
	investment_id::text,
	recurring_payment_id::text,
	created_at,
	updated_at
FROM transactions
`
	args := make([]any, 0, 2)
	if lastID != "" {
		query += "WHERE id > $1::uuid\n"
		args = append(args, lastID)
	}
	query += fmt.Sprintf("ORDER BY id ASC\nLIMIT $%d\n", len(args)+1)
	args = append(args, limit)

	rows, err := j.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]transactionsFormatRow, 0, limit)
	for rows.Next() {
		var row transactionsFormatRow
		if err := rows.Scan(
			&row.ID,
			&row.Status,
			&row.OccurredAt,
			&row.PlannedAt,
			&row.PostedAt,
			&row.CancelledAt,
			&row.BudgetMemberID,
			&row.IncomeSourceID,
			&row.DebtID,
			&row.GoalID,
			&row.InvestmentID,
			&row.RecurringPaymentID,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func buildTransactionFormatPatch(row transactionsFormatRow) transactionFormatPatch {
	patch := transactionFormatPatch{
		PostedAt:    cloneTimePtr(row.PostedAt),
		CancelledAt: cloneTimePtr(row.CancelledAt),
	}

	status := strings.TrimSpace(row.Status)
	switch status {
	case "planned":
		if patch.PostedAt != nil {
			patch.PostedAt = nil
			patch.HasTimestamp = true
		}
		if patch.CancelledAt != nil {
			patch.CancelledAt = nil
			patch.HasTimestamp = true
		}
	case "posted":
		if patch.PostedAt == nil {
			patch.PostedAt = firstNonNilTime(row.OccurredAt, row.UpdatedAt, row.CreatedAt)
			patch.HasTimestamp = true
		}
		if patch.CancelledAt != nil {
			patch.CancelledAt = nil
			patch.HasTimestamp = true
		}
	case "cancelled":
		if patch.CancelledAt == nil {
			patch.CancelledAt = firstNonNilTime(
				row.UpdatedAt,
				row.PostedAt,
				row.OccurredAt,
				row.PlannedAt,
				row.CreatedAt,
			)
			patch.HasTimestamp = true
		}
	}

	if row.BudgetMemberID != nil ||
		row.IncomeSourceID != nil ||
		row.DebtID != nil ||
		row.GoalID != nil ||
		row.InvestmentID != nil ||
		row.RecurringPaymentID != nil {
		patch.HasReservedIDs = true
	}

	patch.NeedsFix = patch.HasTimestamp || patch.HasReservedIDs
	return patch
}

func (j *TransactionsFormatJob) applyPatch(
	ctx context.Context,
	row transactionsFormatRow,
	patch transactionFormatPatch,
	now time.Time,
) (bool, error) {
	const query = `
UPDATE transactions
SET
	posted_at = $2,
	cancelled_at = $3,
	budget_member_id = NULL,
	income_source_id = NULL,
	debt_id = NULL,
	goal_id = NULL,
	investment_id = NULL,
	recurring_payment_id = NULL,
	updated_at = $4
WHERE id = $1::uuid
  AND updated_at = $5
`

	commandTag, err := j.pool.Exec(
		ctx,
		query,
		row.ID,
		patch.PostedAt,
		patch.CancelledAt,
		now,
		row.UpdatedAt,
	)
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() > 0, nil
}

func appendIssue(issues []transactionFormatIssue, issue transactionFormatIssue) []transactionFormatIssue {
	if len(issues) >= maxIssueSamples {
		return issues
	}
	return append(issues, issue)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}

func firstNonNilTime(values ...any) *time.Time {
	for _, value := range values {
		switch typed := value.(type) {
		case *time.Time:
			if typed == nil {
				continue
			}
			copied := typed.UTC()
			return &copied
		case time.Time:
			copied := typed.UTC()
			return &copied
		}
	}
	return nil
}
