package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionsVerificationOptions struct {
	BatchSize           int
	Limit               int
	SampleLimit         int
	BaselineOut         string
	BaselineIn          string
	ReportFile          string
	MaxUpdatedAtRFC3339 string
}

type TransactionsVerificationJob struct {
	pool *pgxpool.Pool
	out  io.Writer
}

type TransactionsVerificationSnapshot struct {
	GeneratedAt time.Time                               `json:"generated_at"`
	Scope       TransactionsVerificationScope           `json:"scope"`
	Totals      TransactionsVerificationTotals          `json:"totals"`
	Aggregates  TransactionsVerificationAggregates      `json:"aggregates"`
	Invariants  TransactionsVerificationInvariantCounts `json:"invariants"`
	Samples     map[string][]string                     `json:"samples,omitempty"`
}

type TransactionsVerificationScope struct {
	Limit        int        `json:"limit"`
	MaxUpdatedAt *time.Time `json:"max_updated_at,omitempty"`
}

type TransactionsVerificationTotals struct {
	Scanned int64 `json:"scanned"`
}

type TransactionsVerificationAggregates struct {
	CountByStatus       map[string]int64 `json:"count_by_status"`
	CountByType         map[string]int64 `json:"count_by_type"`
	CountByCurrency     map[string]int64 `json:"count_by_currency"`
	AmountByCurrency    map[string]int64 `json:"amount_minor_by_currency"`
	AmountByStatus      map[string]int64 `json:"amount_minor_by_status"`
	RowsNeedingRepair   int64            `json:"rows_needing_repair"`
	RowsAlreadyAligned  int64            `json:"rows_already_aligned"`
	RowsWithUnknownEnum int64            `json:"rows_with_unknown_enum"`
}

type TransactionsVerificationInvariantCounts struct {
	PostedWithoutPostedAt       int64 `json:"posted_without_posted_at"`
	PlannedWithPostedAt         int64 `json:"planned_with_posted_at"`
	PlannedWithCancelledAt      int64 `json:"planned_with_cancelled_at"`
	PostedWithCancelledAt       int64 `json:"posted_with_cancelled_at"`
	CancelledWithoutCancelledAt int64 `json:"cancelled_without_cancelled_at"`
	ReservedRefsPresent         int64 `json:"reserved_refs_present"`
	EffectiveAtMissing          int64 `json:"effective_at_missing"`
}

type VerificationSeverity string

const (
	SeverityHigh   VerificationSeverity = "HIGH"
	SeverityMedium VerificationSeverity = "MEDIUM"
	SeverityLow    VerificationSeverity = "LOW"
)

type TransactionsVerificationDiscrepancy struct {
	Code        string               `json:"code"`
	Severity    VerificationSeverity `json:"severity"`
	Description string               `json:"description"`
	Before      string               `json:"before,omitempty"`
	After       string               `json:"after,omitempty"`
	Remediation string               `json:"remediation"`
}

type TransactionsVerificationReport struct {
	GeneratedAt      time.Time                             `json:"generated_at"`
	BaselinePath     string                                `json:"baseline_path,omitempty"`
	BaselineCompared bool                                  `json:"baseline_compared"`
	Snapshot         TransactionsVerificationSnapshot      `json:"snapshot"`
	Discrepancies    []TransactionsVerificationDiscrepancy `json:"discrepancies"`
}

type transactionsVerificationRow struct {
	ID                 string
	Status             string
	Type               string
	Currency           string
	AmountMinor        int64
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
	UpdatedAt          time.Time
	CreatedAt          time.Time
}

func NewTransactionsVerificationJob(pool *pgxpool.Pool, out io.Writer) *TransactionsVerificationJob {
	if out == nil {
		out = io.Discard
	}
	return &TransactionsVerificationJob{
		pool: pool,
		out:  out,
	}
}

func (j *TransactionsVerificationJob) Run(ctx context.Context, opts TransactionsVerificationOptions) error {
	if j == nil || j.pool == nil {
		return fmt.Errorf("transactions verification job is not initialized")
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.SampleLimit <= 0 {
		opts.SampleLimit = 50
	}

	var maxUpdatedAt *time.Time
	if strings.TrimSpace(opts.MaxUpdatedAtRFC3339) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(opts.MaxUpdatedAtRFC3339))
		if err != nil {
			return fmt.Errorf("parse max-updated-at: %w", err)
		}
		utc := parsed.UTC()
		maxUpdatedAt = &utc
	}

	startedAt := time.Now().UTC()
	fmt.Fprintf(
		j.out,
		"verify transactions-format started at=%s batch-size=%d limit=%d sample-limit=%d max-updated-at=%s\n",
		startedAt.Format(time.RFC3339),
		opts.BatchSize,
		opts.Limit,
		opts.SampleLimit,
		timePtrToString(maxUpdatedAt),
	)

	snapshot, err := j.collectSnapshot(ctx, opts, maxUpdatedAt)
	if err != nil {
		return err
	}

	if strings.TrimSpace(opts.BaselineOut) != "" {
		if err := writeVerificationSnapshot(opts.BaselineOut, snapshot); err != nil {
			return fmt.Errorf("write baseline snapshot: %w", err)
		}
		fmt.Fprintf(j.out, "baseline snapshot written to %s\n", opts.BaselineOut)
	}

	var (
		discrepancies []TransactionsVerificationDiscrepancy
		baselinePath  string
	)
	discrepancies = append(discrepancies, discrepanciesFromCurrentSnapshot(snapshot)...)

	if strings.TrimSpace(opts.BaselineIn) != "" {
		baseline, readErr := readVerificationSnapshot(opts.BaselineIn)
		if readErr != nil {
			return fmt.Errorf("read baseline snapshot: %w", readErr)
		}
		baselinePath = opts.BaselineIn
		discrepancies = append(discrepancies, compareSnapshots(baseline, snapshot)...)
	}

	sort.Slice(discrepancies, func(i, k int) bool {
		left := severityRank(discrepancies[i].Severity)
		right := severityRank(discrepancies[k].Severity)
		if left != right {
			return left > right
		}
		return discrepancies[i].Code < discrepancies[k].Code
	})

	report := TransactionsVerificationReport{
		GeneratedAt:      time.Now().UTC(),
		BaselinePath:     baselinePath,
		BaselineCompared: baselinePath != "",
		Snapshot:         snapshot,
		Discrepancies:    discrepancies,
	}

	fmt.Fprintf(
		j.out,
		"verification summary: scanned=%d discrepancies=%d high=%d medium=%d low=%d\n",
		report.Snapshot.Totals.Scanned,
		len(report.Discrepancies),
		countDiscrepanciesBySeverity(report.Discrepancies, SeverityHigh),
		countDiscrepanciesBySeverity(report.Discrepancies, SeverityMedium),
		countDiscrepanciesBySeverity(report.Discrepancies, SeverityLow),
	)

	markdown := renderVerificationReportMarkdown(report)
	if strings.TrimSpace(opts.ReportFile) != "" {
		if err := os.WriteFile(opts.ReportFile, []byte(markdown), 0o644); err != nil {
			return fmt.Errorf("write report file: %w", err)
		}
		fmt.Fprintf(j.out, "verification report written to %s\n", opts.ReportFile)
	}

	fmt.Fprintln(j.out, markdown)
	return nil
}

func (j *TransactionsVerificationJob) collectSnapshot(
	ctx context.Context,
	opts TransactionsVerificationOptions,
	maxUpdatedAt *time.Time,
) (TransactionsVerificationSnapshot, error) {
	snapshot := TransactionsVerificationSnapshot{
		GeneratedAt: time.Now().UTC(),
		Scope: TransactionsVerificationScope{
			Limit:        opts.Limit,
			MaxUpdatedAt: maxUpdatedAt,
		},
		Aggregates: TransactionsVerificationAggregates{
			CountByStatus:    make(map[string]int64),
			CountByType:      make(map[string]int64),
			CountByCurrency:  make(map[string]int64),
			AmountByCurrency: make(map[string]int64),
			AmountByStatus:   make(map[string]int64),
		},
		Samples: make(map[string][]string),
	}

	var lastID string
	for {
		batchLimit := opts.BatchSize
		if opts.Limit > 0 {
			remaining := opts.Limit - int(snapshot.Totals.Scanned)
			if remaining <= 0 {
				break
			}
			if remaining < batchLimit {
				batchLimit = remaining
			}
		}

		rows, err := j.fetchVerificationBatch(ctx, lastID, batchLimit, maxUpdatedAt)
		if err != nil {
			return TransactionsVerificationSnapshot{}, fmt.Errorf("fetch verification batch: %w", err)
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			snapshot.Totals.Scanned++
			lastID = row.ID

			status := strings.TrimSpace(row.Status)
			typ := strings.TrimSpace(row.Type)
			currency := strings.TrimSpace(row.Currency)

			if status == "" || typ == "" || currency == "" {
				snapshot.Aggregates.RowsWithUnknownEnum++
			}

			if status != "" {
				snapshot.Aggregates.CountByStatus[status]++
				snapshot.Aggregates.AmountByStatus[status] += row.AmountMinor
			}
			if typ != "" {
				snapshot.Aggregates.CountByType[typ]++
			}
			if currency != "" {
				snapshot.Aggregates.CountByCurrency[currency]++
				snapshot.Aggregates.AmountByCurrency[currency] += row.AmountMinor
			}

			patch := buildTransactionFormatPatch(transactionsFormatRow{
				ID:                 row.ID,
				Status:             row.Status,
				OccurredAt:         row.OccurredAt,
				PlannedAt:          row.PlannedAt,
				PostedAt:           row.PostedAt,
				CancelledAt:        row.CancelledAt,
				BudgetMemberID:     row.BudgetMemberID,
				IncomeSourceID:     row.IncomeSourceID,
				DebtID:             row.DebtID,
				GoalID:             row.GoalID,
				InvestmentID:       row.InvestmentID,
				RecurringPaymentID: row.RecurringPaymentID,
				CreatedAt:          row.CreatedAt,
				UpdatedAt:          row.UpdatedAt,
			})
			if patch.NeedsFix {
				snapshot.Aggregates.RowsNeedingRepair++
				addSample(snapshot.Samples, "rows_needing_repair", row.ID, opts.SampleLimit)
			} else {
				snapshot.Aggregates.RowsAlreadyAligned++
			}

			switch status {
			case "posted":
				if row.PostedAt == nil {
					snapshot.Invariants.PostedWithoutPostedAt++
					addSample(snapshot.Samples, "posted_without_posted_at", row.ID, opts.SampleLimit)
				}
				if row.CancelledAt != nil {
					snapshot.Invariants.PostedWithCancelledAt++
					addSample(snapshot.Samples, "posted_with_cancelled_at", row.ID, opts.SampleLimit)
				}
			case "planned":
				if row.PostedAt != nil {
					snapshot.Invariants.PlannedWithPostedAt++
					addSample(snapshot.Samples, "planned_with_posted_at", row.ID, opts.SampleLimit)
				}
				if row.CancelledAt != nil {
					snapshot.Invariants.PlannedWithCancelledAt++
					addSample(snapshot.Samples, "planned_with_cancelled_at", row.ID, opts.SampleLimit)
				}
			case "cancelled":
				if row.CancelledAt == nil {
					snapshot.Invariants.CancelledWithoutCancelledAt++
					addSample(snapshot.Samples, "cancelled_without_cancelled_at", row.ID, opts.SampleLimit)
				}
			}

			if row.BudgetMemberID != nil ||
				row.IncomeSourceID != nil ||
				row.DebtID != nil ||
				row.GoalID != nil ||
				row.InvestmentID != nil ||
				row.RecurringPaymentID != nil {
				snapshot.Invariants.ReservedRefsPresent++
				addSample(snapshot.Samples, "reserved_refs_present", row.ID, opts.SampleLimit)
			}

			if row.OccurredAt == nil && row.PlannedAt == nil {
				snapshot.Invariants.EffectiveAtMissing++
				addSample(snapshot.Samples, "effective_at_missing", row.ID, opts.SampleLimit)
			}
		}
	}

	return snapshot, nil
}

func (j *TransactionsVerificationJob) fetchVerificationBatch(
	ctx context.Context,
	lastID string,
	limit int,
	maxUpdatedAt *time.Time,
) ([]transactionsVerificationRow, error) {
	query := `
SELECT
	id::text,
	status,
	type,
	currency,
	amount_minor,
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
	updated_at,
	created_at
FROM transactions
WHERE 1=1
`

	args := make([]any, 0, 3)
	if maxUpdatedAt != nil {
		query += fmt.Sprintf("  AND updated_at <= $%d\n", len(args)+1)
		args = append(args, *maxUpdatedAt)
	}
	if lastID != "" {
		query += fmt.Sprintf("  AND id > $%d::uuid\n", len(args)+1)
		args = append(args, lastID)
	}
	query += fmt.Sprintf("ORDER BY id ASC\nLIMIT $%d\n", len(args)+1)
	args = append(args, limit)

	rows, err := j.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]transactionsVerificationRow, 0, limit)
	for rows.Next() {
		var row transactionsVerificationRow
		if err := rows.Scan(
			&row.ID,
			&row.Status,
			&row.Type,
			&row.Currency,
			&row.AmountMinor,
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
			&row.UpdatedAt,
			&row.CreatedAt,
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

func writeVerificationSnapshot(path string, snapshot TransactionsVerificationSnapshot) error {
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func readVerificationSnapshot(path string) (TransactionsVerificationSnapshot, error) {
	var snapshot TransactionsVerificationSnapshot
	payload, err := os.ReadFile(path)
	if err != nil {
		return snapshot, err
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func discrepanciesFromCurrentSnapshot(snapshot TransactionsVerificationSnapshot) []TransactionsVerificationDiscrepancy {
	discrepancies := make([]TransactionsVerificationDiscrepancy, 0, 8)

	addIfPositive := func(code string, count int64, severity VerificationSeverity, description string, remediation string) {
		if count <= 0 {
			return
		}
		discrepancies = append(discrepancies, TransactionsVerificationDiscrepancy{
			Code:        code,
			Severity:    severity,
			Description: description,
			After:       fmt.Sprintf("%d", count),
			Remediation: remediation,
		})
	}

	addIfPositive(
		"posted_without_posted_at",
		snapshot.Invariants.PostedWithoutPostedAt,
		SeverityHigh,
		"Posted transactions without posted_at",
		"Run repair job transactions-format and verify status timestamp mapping for posted records.",
	)
	addIfPositive(
		"cancelled_without_cancelled_at",
		snapshot.Invariants.CancelledWithoutCancelledAt,
		SeverityHigh,
		"Cancelled transactions without cancelled_at",
		"Run repair job transactions-format and confirm cancelled status timeline reconstruction.",
	)
	addIfPositive(
		"planned_with_posted_at",
		snapshot.Invariants.PlannedWithPostedAt,
		SeverityMedium,
		"Planned transactions contain posted_at",
		"Run repair job transactions-format and review producer that writes planned state.",
	)
	addIfPositive(
		"planned_with_cancelled_at",
		snapshot.Invariants.PlannedWithCancelledAt,
		SeverityMedium,
		"Planned transactions contain cancelled_at",
		"Run repair job transactions-format and validate status transition writers.",
	)
	addIfPositive(
		"posted_with_cancelled_at",
		snapshot.Invariants.PostedWithCancelledAt,
		SeverityMedium,
		"Posted transactions contain cancelled_at",
		"Run repair job transactions-format and audit status mutation path for posted transactions.",
	)
	addIfPositive(
		"reserved_refs_present",
		snapshot.Invariants.ReservedRefsPresent,
		SeverityMedium,
		"Reserved MVP1 reference columns are non-null",
		"Run repair job transactions-format to null reserved refs and block non-null writes in transport/app validators.",
	)
	addIfPositive(
		"effective_at_missing",
		snapshot.Invariants.EffectiveAtMissing,
		SeverityLow,
		"Transactions with both occurred_at and planned_at missing",
		"Backfill effective date for legacy rows or classify records for manual review.",
	)

	return discrepancies
}

func compareSnapshots(
	before TransactionsVerificationSnapshot,
	after TransactionsVerificationSnapshot,
) []TransactionsVerificationDiscrepancy {
	discrepancies := make([]TransactionsVerificationDiscrepancy, 0, 16)

	appendDelta := func(code string, severity VerificationSeverity, description string, beforeValue int64, afterValue int64, remediation string) {
		if beforeValue == afterValue {
			return
		}
		discrepancies = append(discrepancies, TransactionsVerificationDiscrepancy{
			Code:        code,
			Severity:    severity,
			Description: description,
			Before:      fmt.Sprintf("%d", beforeValue),
			After:       fmt.Sprintf("%d", afterValue),
			Remediation: remediation,
		})
	}

	appendDelta(
		"delta_total_scanned",
		SeverityLow,
		"Total scanned rows changed versus baseline",
		before.Totals.Scanned,
		after.Totals.Scanned,
		"Confirm rollout window had writes; for strict before/after comparison use --max-updated-at to freeze cohort.",
	)
	appendDelta(
		"delta_rows_needing_repair",
		SeverityHigh,
		"Rows needing repair changed versus baseline",
		before.Aggregates.RowsNeedingRepair,
		after.Aggregates.RowsNeedingRepair,
		"If increased, re-run repair and investigate new incompatible writers; if not decreased, inspect issue samples and patch data manually where needed.",
	)

	appendMapDiffs(
		&discrepancies,
		"delta_count_by_status",
		SeverityLow,
		"Count by status changed versus baseline",
		before.Aggregates.CountByStatus,
		after.Aggregates.CountByStatus,
		"Validate business activity or freeze cohort with --max-updated-at for deterministic verification.",
	)
	appendMapDiffs(
		&discrepancies,
		"delta_count_by_type",
		SeverityLow,
		"Count by type changed versus baseline",
		before.Aggregates.CountByType,
		after.Aggregates.CountByType,
		"Validate business activity or compare against fixed cohort.",
	)
	appendMapDiffs(
		&discrepancies,
		"delta_amount_by_currency_minor",
		SeverityMedium,
		"Amount by currency (minor units) changed versus baseline",
		before.Aggregates.AmountByCurrency,
		after.Aggregates.AmountByCurrency,
		"Reconcile with transactional activity window; for migration-only verification lock cohort by max updated_at.",
	)

	return discrepancies
}

func appendMapDiffs(
	target *[]TransactionsVerificationDiscrepancy,
	codePrefix string,
	severity VerificationSeverity,
	description string,
	before map[string]int64,
	after map[string]int64,
	remediation string,
) {
	keys := unionKeys(before, after)
	for _, key := range keys {
		beforeValue := before[key]
		afterValue := after[key]
		if beforeValue == afterValue {
			continue
		}
		*target = append(*target, TransactionsVerificationDiscrepancy{
			Code:        fmt.Sprintf("%s.%s", codePrefix, key),
			Severity:    severity,
			Description: description + " (" + key + ")",
			Before:      fmt.Sprintf("%d", beforeValue),
			After:       fmt.Sprintf("%d", afterValue),
			Remediation: remediation,
		})
	}
}

func unionKeys(left map[string]int64, right map[string]int64) []string {
	merged := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		merged[key] = struct{}{}
	}
	for key := range right {
		merged[key] = struct{}{}
	}
	result := make([]string, 0, len(merged))
	for key := range merged {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func renderVerificationReportMarkdown(report TransactionsVerificationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Migration Verification Report\n\n")
	fmt.Fprintf(&b, "- Generated at: `%s`\n", report.GeneratedAt.Format(time.RFC3339))
	if report.BaselineCompared {
		fmt.Fprintf(&b, "- Baseline: `%s`\n", report.BaselinePath)
	} else {
		fmt.Fprintf(&b, "- Baseline: `not provided`\n")
	}
	fmt.Fprintf(&b, "- Scope limit: `%d`\n", report.Snapshot.Scope.Limit)
	fmt.Fprintf(&b, "- Scope max updated at: `%s`\n", timePtrToString(report.Snapshot.Scope.MaxUpdatedAt))
	fmt.Fprintf(&b, "- Total scanned: `%d`\n\n", report.Snapshot.Totals.Scanned)

	fmt.Fprintf(&b, "## Snapshot\n\n")
	fmt.Fprintf(
		&b,
		"- Rows needing repair: `%d`\n- Rows already aligned: `%d`\n- Rows with unknown enum: `%d`\n\n",
		report.Snapshot.Aggregates.RowsNeedingRepair,
		report.Snapshot.Aggregates.RowsAlreadyAligned,
		report.Snapshot.Aggregates.RowsWithUnknownEnum,
	)

	fmt.Fprintf(&b, "### Invariants\n\n")
	fmt.Fprintf(
		&b,
		"- posted_without_posted_at: `%d`\n- cancelled_without_cancelled_at: `%d`\n- planned_with_posted_at: `%d`\n- planned_with_cancelled_at: `%d`\n- posted_with_cancelled_at: `%d`\n- reserved_refs_present: `%d`\n- effective_at_missing: `%d`\n\n",
		report.Snapshot.Invariants.PostedWithoutPostedAt,
		report.Snapshot.Invariants.CancelledWithoutCancelledAt,
		report.Snapshot.Invariants.PlannedWithPostedAt,
		report.Snapshot.Invariants.PlannedWithCancelledAt,
		report.Snapshot.Invariants.PostedWithCancelledAt,
		report.Snapshot.Invariants.ReservedRefsPresent,
		report.Snapshot.Invariants.EffectiveAtMissing,
	)

	fmt.Fprintf(&b, "## Discrepancies\n\n")
	if len(report.Discrepancies) == 0 {
		fmt.Fprintf(&b, "No discrepancies found.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "| Code | Severity | Before | After | Description | Remediation |\n")
	fmt.Fprintf(&b, "|---|---|---:|---:|---|---|\n")
	for _, item := range report.Discrepancies {
		fmt.Fprintf(
			&b,
			"| `%s` | `%s` | `%s` | `%s` | %s | %s |\n",
			item.Code,
			item.Severity,
			emptyToDash(item.Before),
			emptyToDash(item.After),
			item.Description,
			item.Remediation,
		)
	}

	if len(report.Snapshot.Samples) > 0 {
		fmt.Fprintf(&b, "\n## Issue samples\n\n")
		keys := make([]string, 0, len(report.Snapshot.Samples))
		for key := range report.Snapshot.Samples {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "- `%s`: `%s`\n", key, strings.Join(report.Snapshot.Samples[key], ", "))
		}
	}

	return b.String()
}

func addSample(samples map[string][]string, code string, id string, sampleLimit int) {
	values := samples[code]
	if len(values) >= sampleLimit {
		return
	}
	samples[code] = append(values, id)
}

func timePtrToString(value *time.Time) string {
	if value == nil {
		return "not-set"
	}
	return value.UTC().Format(time.RFC3339)
}

func severityRank(severity VerificationSeverity) int {
	switch severity {
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	default:
		return 1
	}
}

func countDiscrepanciesBySeverity(items []TransactionsVerificationDiscrepancy, severity VerificationSeverity) int {
	count := 0
	for _, item := range items {
		if item.Severity == severity {
			count++
		}
	}
	return count
}

func emptyToDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
