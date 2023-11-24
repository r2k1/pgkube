package queries

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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

// poor man immutable slices
func AllowedGroupBy() []string {
	return []string{"namespace", "controller_kind", "controller_name", "pod_name", "node_name"}
}
func AllowedSortBy() []string {
	return []string{
		"namespace",
		"controller_kind",
		"controller_name",
		"pod_name",
		"node_name",
		"request_cpu_cores",
		"used_cpu_cores",
		"request_memory_bytes_avg",
		"used_memory_bytes_avg",
		"pod_hours",
		"cpu_cost",
		"memory_cost",
		"total_cost",
	}
}

type WorkloadAggRequest struct {
	GroupBy []string
	OderBy  string
	Start   time.Time
	End     time.Time
}

func sortGroupBy(data []string) {
	indexMap := make(map[string]int)
	for i, v := range AllowedGroupBy() {
		indexMap[v] = i
	}

	// Custom sorting function
	sort.Slice(data, func(i, j int) bool {
		return indexMap[data[i]] < indexMap[data[j]]
	})
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
	for _, g := range w.GroupBy {
		if !Contains(AllowedGroupBy(), g) {
			return fmt.Errorf("invalid group by: %s", g)
		}
	}

	orderByCol := strings.TrimPrefix(strings.TrimSuffix(w.OderBy, " desc"), " asc")
	if !Contains(AllowedSortBy(), orderByCol) {
		return fmt.Errorf("invalid order by: %s", w.OderBy)
	}
	return nil
}

func workloadQuery(req WorkloadAggRequest) (string, []interface{}, error) {
	sortGroupBy(req.GroupBy)
	psq := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	hours := req.End.Sub(req.Start).Hours()
	if hours < 0 {
		return "", nil, fmt.Errorf("start time is after end time")
	}

	cols := append([]string(nil), req.GroupBy...)
	cols = append(cols,
		fmt.Sprintf("round((sum(request_cpu_cores * pod_hours) / %f)::numeric, 2) as request_cpu_cores", hours),
		fmt.Sprintf("round((sum(cpu_cores_avg * pod_hours) / %f)::numeric, 2) as used_cpu_cores", hours),
		fmt.Sprintf("round(sum(request_memory_bytes * pod_hours) / %f) as request_memory_bytes_avg", hours),
		fmt.Sprintf("round(sum(memory_bytes_avg * pod_hours) / %f) as used_memory_bytes_avg", hours),
		"round(sum(pod_hours), 2) as pod_hours",
		"round(sum(cpu_cost)::numeric, 2) as cpu_cost",
		"round(sum(memory_cost)::numeric, 2) as memory_cost",
		"round(sum(memory_cost + cpu_cost)::numeric, 2) as total_cost",
	)

	query := psq.
		Select(cols...).
		GroupBy(req.GroupBy...).
		From("cost_pod_hourly").
		Where(sq.GtOrEq{"timestamp": req.Start}).
		Where(sq.LtOrEq{"timestamp": req.End})
	if req.OderBy != "" {
		query = query.OrderBy(req.OderBy)
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
	rows, err := q.db.Query(ctx, sql, args...)
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
