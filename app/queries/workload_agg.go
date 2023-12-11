package queries

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Workload struct {
	Namespace string
	Name      string
}

func Cols() []string {
	return []string{
		"timestamp",
		"date",
		"cluster",
		"namespace",
		"controller_kind",
		"controller_name",
		"name",
		"node_name",
		"request_cpu_cores",
		"used_cpu_cores",
		"request_memory_bytes",
		"used_memory_bytes",
		"request_storage_bytes",
		"hours",
		"cpu_cost",
		"memory_cost",
		"storage_cost",
		"total_cost",
	}
}

type WorkloadAggRequest struct {
	Cols    []string
	OrderBy string
	Start   time.Time
	End     time.Time
}

func Contains(data []string, term string) bool {
	for _, v := range data {
		if v == term {
			return true
		}
	}
	return false
}

// nolint:cyclop
func (w WorkloadAggRequest) Validate() error {
	for _, g := range w.Cols {
		if !Contains(Cols(), g) {
			return fmt.Errorf("invalid column: %s", g)
		}
	}

	orderByCol := strings.TrimPrefix(strings.TrimSuffix(w.OrderBy, " desc"), " asc")
	if !Contains(Cols(), orderByCol) {
		return fmt.Errorf("invalid order by: %s", w.OrderBy)
	}
	return nil
}

var labelRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-_.]*[a-zA-Z0-9]$`)

func workloadQuery(req WorkloadAggRequest) (string, []interface{}, error) {
	psq := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	hours := req.End.Sub(req.Start).Hours()
	if hours < 0 {
		return "", nil, fmt.Errorf("start time is after end time")
	}
	selectMap := map[string]string{
		"timestamp":             "timestamp at time zone 'utc'",
		"date":                  "timestamp::date::text",
		"cluster":               "(select name from cluster where id = cluster_id)",
		"namespace":             "namespace",
		"controller_kind":       "controller_kind",
		"controller_name":       "controller_name",
		"name":                  "name",
		"node_name":             "node_name",
		"request_cpu_cores":     fmt.Sprintf("round((sum(request_cpu_cores * hours) / %f)::numeric, 2)", hours),
		"used_cpu_cores":        fmt.Sprintf("round((sum(cpu_cores_avg * hours) / %f)::numeric, 2)", hours),
		"request_memory_bytes":  fmt.Sprintf("round(sum(request_memory_bytes * hours) / %f)", hours),
		"used_memory_bytes":     fmt.Sprintf("round(sum(memory_bytes_avg * hours) / %f)", hours),
		"request_storage_bytes": fmt.Sprintf("round(sum(request_storage_bytes * hours) / %f)", hours),
		"hours":                 "round(sum(hours), 2)",
		"cpu_cost":              "round(sum(cpu_cost)::numeric, 2)",
		"memory_cost":           "round(sum(memory_cost)::numeric, 2)",
		"storage_cost":          "round(sum(storage_cost)::numeric, 2)",
		"total_cost":            "round(sum(memory_cost + cpu_cost + storage_cost)::numeric, 2)",
	}
	groupByCols := map[string]struct{}{
		"timestamp":       {},
		"date":            {},
		"cluster":         {},
		"namespace":       {},
		"controller_kind": {},
		"controller_name": {},
		"name":            {},
		"node_name":       {},
	}
	selectStmts := make([]string, 0)
	groupByStmts := make([]string, 0)
	// keep column order consistent
	for _, c := range Cols() {
		if Contains(req.Cols, c) {
			selectStmts = append(selectStmts, selectMap[c]+" as "+c)
			if _, ok := groupByCols[c]; ok {
				groupByStmts = append(groupByStmts, selectMap[c])
			}
		}
	}
	for _, c := range req.Cols {
		if !strings.HasPrefix(c, "label_") {
			continue
		}
		label := strings.TrimPrefix(c, "label_")
		// IMPORTANT. Protection from SQL injection
		if !labelRegex.MatchString(label) {
			return "", nil, fmt.Errorf("invalid label: %s", label)
		}
		selectStmts = append(selectStmts, fmt.Sprintf("coalesce(labels->>'%s', '') as %s", label, c))
		groupByStmts = append(groupByStmts, fmt.Sprintf("labels->>'%s'", label))
	}
	query := psq.
		Select(selectStmts...).
		GroupBy(groupByStmts...).
		From("cost_hourly").
		Where(sq.GtOrEq{"timestamp": req.Start}).
		Where(sq.Lt{"timestamp": req.End})
	if req.OrderBy != "" {
		query = query.OrderBy(req.OrderBy)
	}
	// nolint: wrapcheck
	return query.ToSql()
}

type WorkloadAggResult struct {
	Columns      []string
	Rows         [][]string
	SQLQuery     string
	SQLQueryArgs []interface{}
}

func (q *Queries) WorkloadAgg(ctx context.Context, req WorkloadAggRequest) (*WorkloadAggResult, error) {
	sql, args, err := workloadQuery(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build workload aggregation query: %w", err)
	}
	slog.Info("SQL", "sql", sql, "args", args)
	rows, err := q.query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := WorkloadAggResult{
		SQLQuery:     sql,
		SQLQueryArgs: args,
	}
	for _, field := range rows.FieldDescriptions() {
		result.Columns = append(result.Columns, field.Name)
	}
	result.Rows, err = scanRows(rows)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func scanRows(rows pgx.Rows) ([][]string, error) {
	var err error
	result := make([][]string, 0)
	rows.RawValues()
	for rows.Next() {
		rowVals := make([]interface{}, len(rows.FieldDescriptions()))
		for i := range rowVals {
			rowVals[i] = &rowVals[i]
		}
		err = rows.Scan(rowVals...)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rows for workload aggregation query: %w", err)
		}
		// convert all numeric types to float to simplify rendering
		for i := range rowVals {
			numeric, ok := rowVals[i].(pgtype.Numeric)
			if ok {
				fl, err := numeric.Float64Value()
				if err != nil {
					slog.Debug("failed to convert numeric to float64", "error", err)
				}
				rowVals[i] = fl.Float64
			}
		}
		strVals := make([]string, len(rowVals))
		for i := range rowVals {
			strVals[i] = fmtCell(rowVals[i])
		}
		result = append(result, strVals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan rows for workload aggregation query: %w", err)
	}
	return result, nil
}

func fmtCell(v any) string {
	switch v := v.(type) {
	case float64:
		formatted := fmt.Sprintf("%.2f", v)                              // format with two decimal places
		return strings.TrimRight(strings.TrimRight(formatted, "0"), ".") // trim trailing zeros and dot
	case pgtype.Numeric:
		fl, _ := v.Float64Value()
		formatted := fmt.Sprintf("%.2f", fl.Float64)                     // format with two decimal places
		return strings.TrimRight(strings.TrimRight(formatted, "0"), ".") // trim trailing zeros and dot
	default:
		return fmt.Sprint(v)
	}
}
