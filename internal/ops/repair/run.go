package repair

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	commandTransactionsFormat = "transactions-format"
	commandTransactionsVerify = "transactions-verify"
	defaultBatchSize          = 500
	maxBatchSize              = 5000
)

type Runner struct {
	pool *pgxpool.Pool
	out  io.Writer
}

func NewRunner(pool *pgxpool.Pool, out io.Writer) *Runner {
	if out == nil {
		out = io.Discard
	}
	return &Runner{
		pool: pool,
		out:  out,
	}
}

func (r *Runner) Run(ctx context.Context, args []string) error {
	if r == nil || r.pool == nil {
		return errors.New("repair runner is not initialized")
	}
	if len(args) == 0 {
		return r.UsageError()
	}

	switch args[0] {
	case commandTransactionsFormat:
		opts, err := parseTransactionsFormatOptions(args[1:])
		if err != nil {
			return err
		}
		job := NewTransactionsFormatJob(r.pool, r.out)
		return job.Run(ctx, opts)
	case commandTransactionsVerify:
		opts, err := parseTransactionsVerifyOptions(args[1:])
		if err != nil {
			return err
		}
		job := NewTransactionsVerificationJob(r.pool, r.out)
		return job.Run(ctx, opts)
	default:
		return r.UsageError()
	}
}

func (r *Runner) UsageError() error {
	return errors.New(
		"usage: go run ./cmd/ops repair <command> [flags]\n" +
			"commands:\n" +
			"  transactions-format  --dry-run --batch-size=500 --limit=0\n" +
			"  transactions-verify  --batch-size=500 --limit=0 --sample-limit=50 --baseline-out=<file> --baseline-in=<file> --report-file=<file> --max-updated-at=<rfc3339>",
	)
}

func parseTransactionsFormatOptions(args []string) (TransactionsFormatOptions, error) {
	fs := flag.NewFlagSet(commandTransactionsFormat, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := TransactionsFormatOptions{}
	fs.BoolVar(&opts.DryRun, "dry-run", false, "validate and report only, without writing updates")
	fs.IntVar(&opts.BatchSize, "batch-size", defaultBatchSize, "number of rows per batch")
	fs.IntVar(&opts.Limit, "limit", 0, "maximum rows to scan (0 means unlimited)")

	if err := fs.Parse(args); err != nil {
		return TransactionsFormatOptions{}, fmt.Errorf("parse %s options: %w", commandTransactionsFormat, err)
	}
	if fs.NArg() > 0 {
		return TransactionsFormatOptions{}, fmt.Errorf("unexpected extra arguments: %v", fs.Args())
	}
	if opts.BatchSize <= 0 {
		return TransactionsFormatOptions{}, errors.New("batch-size must be > 0")
	}
	if opts.BatchSize > maxBatchSize {
		return TransactionsFormatOptions{}, fmt.Errorf("batch-size must be <= %d", maxBatchSize)
	}
	if opts.Limit < 0 {
		return TransactionsFormatOptions{}, errors.New("limit must be >= 0")
	}

	return opts, nil
}

func parseTransactionsVerifyOptions(args []string) (TransactionsVerificationOptions, error) {
	fs := flag.NewFlagSet(commandTransactionsVerify, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := TransactionsVerificationOptions{}
	fs.IntVar(&opts.BatchSize, "batch-size", defaultBatchSize, "number of rows per batch")
	fs.IntVar(&opts.Limit, "limit", 0, "maximum rows to scan (0 means unlimited)")
	fs.IntVar(&opts.SampleLimit, "sample-limit", 50, "maximum discrepancy samples per class")
	fs.StringVar(&opts.BaselineOut, "baseline-out", "", "path to write baseline snapshot JSON")
	fs.StringVar(&opts.BaselineIn, "baseline-in", "", "path to read baseline snapshot JSON")
	fs.StringVar(&opts.ReportFile, "report-file", "", "path to write markdown verification report")
	fs.StringVar(&opts.MaxUpdatedAtRFC3339, "max-updated-at", "", "optional upper bound for updated_at (RFC3339), useful for stable before/after cohorts")

	if err := fs.Parse(args); err != nil {
		return TransactionsVerificationOptions{}, fmt.Errorf("parse %s options: %w", commandTransactionsVerify, err)
	}
	if fs.NArg() > 0 {
		return TransactionsVerificationOptions{}, fmt.Errorf("unexpected extra arguments: %v", fs.Args())
	}
	if opts.BatchSize <= 0 {
		return TransactionsVerificationOptions{}, errors.New("batch-size must be > 0")
	}
	if opts.BatchSize > maxBatchSize {
		return TransactionsVerificationOptions{}, fmt.Errorf("batch-size must be <= %d", maxBatchSize)
	}
	if opts.Limit < 0 {
		return TransactionsVerificationOptions{}, errors.New("limit must be >= 0")
	}
	if opts.SampleLimit <= 0 {
		return TransactionsVerificationOptions{}, errors.New("sample-limit must be > 0")
	}

	return opts, nil
}
