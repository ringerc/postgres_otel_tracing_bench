package modes

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode2a --- multi-statement simple Query:
//
//	"SET LOCAL otel_api.traceparent='...'; <SQL1>; <SQL2>; ...;"
//
// sent as a single 'Q' message via pgconn.Exec. Postgres treats a multi-
// statement simple Q as one implicit transaction, so SET LOCAL works.
//
// Cost: forces simple protocol. No parameter binding --- every $1/$2/...
// in the workload's SQL is client-side-interpolated into the SQL text,
// no statement cache. Server pays Parse + plan on every iteration.
//
// Wire shape: 1 client message, 1 RTT.
type mode2a struct{}

func (mode2a) ID() string { return "2a" }
func (mode2a) Description() string {
	return "Multi-statement simple Q: SET LOCAL ...; <SQL>;  (1 RTT, no statement cache)"
}
func (mode2a) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode2a) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }

func (mode2a) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	stmts := w.NextStatements()
	if len(stmts) == 0 {
		return 0, errors.New("workload produced no statements")
	}

	var sb strings.Builder
	sb.WriteString("SET LOCAL otel_api.traceparent = '")
	sb.WriteString(tc.Traceparent())
	sb.WriteString("';")
	for _, s := range stmts {
		sql, err := interpolate(s.SQL, s.Args)
		if err != nil {
			return 0, err
		}
		sb.WriteString(sql)
		sb.WriteString(";")
	}

	// pgconn.Conn.Exec issues a single simple 'Q' frame; the returned
	// MultiResultReader walks the per-statement result sets.
	mrr := conn.PgConn().Exec(ctx, sb.String())
	defer mrr.Close()

	total := 0
	for mrr.NextResult() {
		// Drain rows for this statement. ReadAll consumes everything
		// and returns once the per-statement CommandComplete arrives.
		rr := mrr.ResultReader()
		for rr.NextRow() {
			total++
		}
		if _, err := rr.Close(); err != nil {
			return total, err
		}
	}
	if err := mrr.Close(); err != nil {
		return total, err
	}
	// First statement is SET LOCAL which returns no rows; total counts
	// only the workload statements' rows.
	return total, nil
}

// interpolate substitutes positional placeholders ($1, $2, ...) in sql
// with literal forms of args, for use in the simple-protocol path that
// can't carry binds. Supports the subset of types the benchmark
// workloads use: int64 / int, string, []byte.
//
// This is NOT a general SQL escape function. It's safe here because:
//   - the benchmark controls its own SQL text and args,
//   - args come from generated integers and known-shape strings, and
//   - the only string values that flow through are SQL constants from
//     this package or hex-validated trace_id components.
//
// If a future workload adds user-influenceable args, this needs proper
// escaping (pgx exposes pgconn's SanitizeSQL but it's not public API).
func interpolate(sql string, args []any) (string, error) {
	if len(args) == 0 {
		return sql, nil
	}
	var sb strings.Builder
	for i := 0; i < len(sql); i++ {
		if sql[i] != '$' {
			sb.WriteByte(sql[i])
			continue
		}
		// parse the index after '$'
		j := i + 1
		for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
			j++
		}
		if j == i+1 {
			sb.WriteByte('$')
			continue
		}
		idx, _ := strconv.Atoi(sql[i+1 : j])
		if idx < 1 || idx > len(args) {
			return "", errors.New("placeholder $" + sql[i+1:j] + " out of range")
		}
		lit, err := literalize(args[idx-1])
		if err != nil {
			return "", err
		}
		sb.WriteString(lit)
		i = j - 1
	}
	return sb.String(), nil
}

func literalize(v any) (string, error) {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", nil
	default:
		return "", errors.New("unsupported arg type for mode2a interpolation")
	}
}
