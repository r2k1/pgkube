package queries

import (
	"context"
	"errors"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
)

type Workload struct {
	Namespace string
	Name      string
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  []string
	Start   time.Time
	End     time.Time
}

var AllowedGroupBy = []string{"namespace", "controller_kind", "controller_name", "pod_name", "node_name"}

func (w *WorkloadRequest) Validate() error {
	for _, g := range w.GroupBy {
		found := false
		for _, a := range AllowedGroupBy {
			if g == a {
				found = true
				break
			}
		}
		if !found {
			return errors.New("invalid group by")
		}
	}
	return nil
}

func WorkloadQuery(ctx context.Context, req WorkloadRequest) (string, []interface{}, error) {
	cols := append(req.GroupBy,
		"round(sum(memory_bytes_avg * cost_pod_hourly.pod_hours) / sum(pod_hours)) as memory_bytes_avg",
		"round(max(memory_bytes_max)) as memory_bytes_max",
	)
	return sq.Select(cols...).GroupBy(req.GroupBy...).From("cost_pod_hourly").OrderBy(req.OderBy...).ToSql()
}

type WorkloadAggResult struct {
	Columns      []string
	Rows         [][]interface{}
	SQLQuery     string
	SQLQueryArgs []interface{}
}

func (q *Queries) WorkloadAgg(ctx context.Context, req WorkloadRequest) (*WorkloadAggResult, error) {
	if len(req.GroupBy) == 0 {
		req.GroupBy = AllowedGroupBy
	}
	sql, args, err := WorkloadQuery(ctx, req)
	if err != nil {
		return nil, err
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
	for rows.Next() {
		rowVals := make([]interface{}, len(rows.FieldDescriptions()))
		for i := range rowVals {
			rowVals[i] = &rowVals[i]
		}
		err = rows.Scan(rowVals...)
		if err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, rowVals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &result, nil
}
