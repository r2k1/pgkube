package queries

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
)

type Workload struct {
	Namespace string
	Name      string
}

// don't accese these variables directly, use the functions below
var allowedGroupBy = []string{"namespace", "controller_kind", "controller_name", "pod_name", "node_name"}
var allowedSortBy = []string{"namespace", "controller_kind", "controller_name", "pod_name", "node_name"}

// poor man immutable slices
func AllowedGroupBy() []string {
	return append([]string(nil), allowedGroupBy...)
}
func AllowedSortBy() []string {
	return append([]string(nil), allowedSortBy...)
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  []string
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

func (w WorkloadRequest) Validate() error {
	for _, g := range w.GroupBy {
		found := false
		for _, a := range AllowedGroupBy() {
			if g == a {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid group by: %s", g)
		}
	}

	for _, o := range w.OderBy {
		found := false
		for _, a := range AllowedSortBy() {
			if o == a || strings.TrimSuffix(o, " desc") == a || strings.TrimSuffix(o, " asc") == a {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid order by: %s", o)
		}
	}

	return nil
}

func WorkloadQuery(req WorkloadRequest) (string, []interface{}, error) {
	sortGroupBy(req.GroupBy)
	cols := append(req.GroupBy,
		"round(sum(memory_bytes_avg * pod_hours) / sum(pod_hours)) as memory_bytes_avg",
		"round(max(memory_bytes_max)) as memory_bytes_max",
	)
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar).
		Select(cols...).
		GroupBy(req.GroupBy...).
		From("cost_pod_hourly").
		Where(sq.GtOrEq{"timestamp": req.Start}).
		Where(sq.LtOrEq{"timestamp": req.End}).
		OrderBy(req.OderBy...).
		ToSql()
}

type WorkloadAggResult struct {
	Columns      []string
	Rows         [][]interface{}
	SQLQuery     string
	SQLQueryArgs []interface{}
}

func (q *Queries) WorkloadAgg(ctx context.Context, req WorkloadRequest) (*WorkloadAggResult, error) {
	sql, args, err := WorkloadQuery(req)
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
