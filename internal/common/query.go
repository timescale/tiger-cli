package common

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// Column is a column in a query result set.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResultSet is the result of a single statement. Rows is a pointer so a
// statement that returns no rows at all (DDL, INSERT/UPDATE/DELETE) omits the
// field entirely, while a SELECT that matched nothing still reports an empty
// list.
type ResultSet struct {
	CommandTag   string       `json:"command_tag"`
	Columns      []Column     `json:"columns,omitempty"`
	Rows         *[][]*string `json:"rows,omitempty"`
	RowsAffected int64        `json:"rows_affected"`
	Truncated    bool         `json:"truncated,omitempty"`
}

// QueryResult is the complete result of a query execution. Callers render it
// themselves (a table for the CLI, structured output for MCP), so it carries no
// JSON tags.
type QueryResult struct {
	ResultSets    []ResultSet
	ExecutionTime time.Duration
	Truncated     bool
}

// ExecuteQueryArgs configures a call to ExecuteQuery.
type ExecuteQueryArgs struct {
	Query      string
	Parameters []string
	Role       string
	Pooled     bool

	// ReadOnly opens the session in Tiger Cloud's immutable read-only mode.
	ReadOnly bool

	// MaxRows caps the rows returned per result set and MaxBytes caps the
	// approximate serialized size of all rows returned. Zero means no cap.
	// Only the MCP tool sets them, to bound how much data reaches an agent's
	// context; the CLI returns everything the query produced.
	MaxRows  int
	MaxBytes int
}

// ExecuteQuery runs a query against a service (or one of its read replicas) and
// collects every result set it produces. It is the shared entry point for the
// `tiger db query` CLI command and the db_query MCP tool.
//
// Multi-statement queries (semicolon-separated) are supported when no
// parameters are given; with parameters, only a single statement is.
//
// Declared as a var so tests can replace it with a stub that doesn't require a
// real database connection.
var ExecuteQuery = func(ctx context.Context, cfg *config.Config, target *ConnectionTarget, args ExecuteQueryArgs) (*QueryResult, error) {
	// Fail with "service is paused"/"service is not ready" rather than an
	// opaque connection error.
	if err := CheckServiceReady(target.ConnectionService); err != nil {
		return nil, err
	}

	details, err := target.Details(cfg, ConnectionDetailsOptions{
		Pooled:       args.Pooled,
		Role:         args.Role,
		WithPassword: true,
		ReadOnly:     args.ReadOnly,
	})
	if err != nil {
		return nil, err
	}

	connConfig, err := pgx.ParseConfig(details.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Choose the query execution mode based on whether parameters are present.
	// The simple protocol supports multi-statement queries but interpolates
	// parameters client-side (which we don't want to do, for security's sake).
	// The extended protocol sends parameters separately but doesn't support
	// multi-statement queries. This means we don't support multi-statement
	// queries with parameters (pgx returns an error for them when using
	// QueryExecModeExec). See [pgx.QueryExecMode] for details.
	if len(args.Parameters) > 0 {
		// QueryExecModeExec rather than QueryExecModeDescribeExec: DescribeExec
		// asks for each column in its preferred binary format, but we scan
		// every column into *string, which only works with the text format.
		// Exec uses text results and skips the extra describe round trip.
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	} else {
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(context.Background())

	// Measure only the query itself, not the connection setup.
	startTime := time.Now()

	// Queue the query. When using QueryExecModeSimpleProtocol (no parameters),
	// it's valid to queue a single multi-statement SQL query as the batch.
	// See the [pgx.Batch.Queue] documentation for details. When using
	// QueryExecModeExec (with parameters), queueing a multi-statement query
	// here will result in an error when executing it below.
	batch := &pgx.Batch{}
	batch.Queue(args.Query, util.ConvertSliceToAny(args.Parameters)...)

	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	// A nil budget means unlimited; processResultSet decrements it in place so
	// the cap spans every result set in the batch.
	var remainingBytes *int
	if args.MaxBytes > 0 {
		remainingBytes = &args.MaxBytes
	}

	resultSets := make([]ResultSet, 0)
	truncated := false
	for {
		rows, err := br.Query()
		if err != nil {
			// Check if we've reached the final result set and stop iteration.
			// NOTE: It would be nice if there was a real sentinel error type
			// we could check here instead of comparing error strings, but pgx
			// doesn't expose one. We will just need to verify that the error
			// message doesn't change when we update the pgx dependency.
			if err.Error() == "no more results in batch" {
				break
			}
			return nil, err
		}

		result, err := processResultSet(conn, rows, args.MaxRows, remainingBytes)
		if err != nil {
			return nil, err
		}

		resultSets = append(resultSets, result)

		if result.Truncated {
			// Stop reading further sets; br.Close() below discards them. The
			// query isn't cancelled, so all statements still run server-side.
			truncated = true
			break
		}
	}

	// Close the batch, discarding any result sets we didn't read.
	if err := br.Close(); err != nil {
		return nil, err
	}

	return &QueryResult{
		ResultSets:    resultSets,
		ExecutionTime: time.Since(startTime),
		Truncated:     truncated,
	}, nil
}

// processResultSet reads one result set, stopping at maxRows or when the shared
// byte budget runs out (either cap disabled by a zero maxRows / nil budget).
// ResultSet.Truncated reports whether rows were left unread.
func processResultSet(conn *pgx.Conn, rows pgx.Rows, maxRows int, remainingBytes *int) (ResultSet, error) {
	defer rows.Close()

	// Get column metadata from field descriptions
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]Column, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		// Get the type name from the connection's type map
		typeName := "unknown"
		dataType, ok := conn.TypeMap().TypeForOID(fd.DataTypeOID)
		if ok && dataType != nil {
			typeName = dataType.Name
		}
		columns[i] = Column{
			Name: fd.Name,
			Type: typeName,
		}
	}

	// Collect rows from this result set
	var resultRows [][]*string
	if len(columns) > 0 {
		// If any columns were returned, initialize resultRows to an empty
		// slice to ensure we always return a list in the results, even if
		// empty (we want to be completely clear when a SELECT query returns no
		// rows). On the other hand, if no columns were returned, it's not a
		// result returning query (e.g. it's DDL or an INSERT/UPDATE/DELETE/etc.),
		// so we leave resultRows nil so it gets omitted from the result.
		resultRows = make([][]*string, 0)
	}

	truncated := false
	for rows.Next() {
		// Row cap: another row exists but we already hold maxRows.
		if maxRows > 0 && len(resultRows) >= maxRows {
			truncated = true
			break
		}

		// Scan every column into a *string so values come back exactly as
		// Postgres rendered them in its text format, with NULL as nil. Letting
		// pgx decode into Go types instead loses or mangles anything without a
		// faithful JSON form: uuid becomes a 16-element byte array, interval
		// and bit become structs of pgx internals, macaddr becomes base64, an
		// int8 past 2^53 loses precision, and float8 'Infinity' can't be
		// marshaled at all.
		values := make([]*string, len(columns))
		scanDest := make([]any, len(columns))
		for i := range values {
			scanDest[i] = &values[i]
		}
		if err := rows.Scan(scanDest...); err != nil {
			return ResultSet{}, fmt.Errorf("failed to scan row: %w", err)
		}

		// Byte safety net for wide rows the row cap alone would miss, but
		// always keep at least one row so an oversized first row doesn't yield
		// an empty result.
		if remainingBytes != nil {
			*remainingBytes -= approxRowSize(values)
			if len(resultRows) > 0 && *remainingBytes < 0 {
				truncated = true
				break
			}
		}

		resultRows = append(resultRows, values)
	}

	// Drain so the command tag reports the true row count even when truncated.
	rows.Close()

	if err := rows.Err(); err != nil {
		return ResultSet{}, err
	}

	commandTag := rows.CommandTag()

	return ResultSet{
		CommandTag:   commandTag.String(),
		Columns:      columns,
		Rows:         util.PtrIfNonNil(resultRows),
		RowsAffected: commandTag.RowsAffected(),
		Truncated:    truncated,
	}, nil
}

// approxRowSize estimates a row's serialized size in bytes for the byte budget,
// mirroring how it is ultimately marshaled to JSON for the client.
func approxRowSize(values []*string) int {
	if b, err := json.Marshal(values); err == nil {
		return len(b)
	}
	// Fallback for the rare value that isn't JSON-marshalable.
	size := 0
	for _, v := range values {
		size += len(util.Deref(v))
	}
	return size
}
