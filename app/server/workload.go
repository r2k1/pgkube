package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/r2k1/pgkube/app/queries"
)

type HomeTemplateData struct {
	Request          WorkloadRequest
	AggData          *queries.WorkloadAggResult
	GroupByOptions   []string
	TimeRangeOptions []TimeRangeOptions
}

type TimeRangeOptions struct {
	Label string
	Value string
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  string
	Range   string
	Start   string
	End     string
}

func DefaultRequest() WorkloadRequest {
	return WorkloadRequest{
		GroupBy: []string{"namespace", "controller_kind", "controller_name"},
		OderBy:  "namespace",
		Range:   "168h",
	}
}

func (r WorkloadRequest) ToQuery() (queries.WorkloadAggRequest, error) {
	if r.Range != "" && (r.Start != "" || r.End != "") {
		return queries.WorkloadAggRequest{}, fmt.Errorf("range and start/end are mutually exclusive")
	}
	var start, end time.Time
	if r.Range != "" {
		end = time.Now().Truncate(time.Hour * 24)
		duration, err := time.ParseDuration(r.Range)
		if err != nil {
			return queries.WorkloadAggRequest{}, fmt.Errorf("invalid range: %w", err)
		}
		start = end.Add(-duration)
	}
	return queries.WorkloadAggRequest{
		GroupBy: r.GroupBy,
		OderBy:  r.OderBy,
		Start:   start,
		End:     end,
	}, nil
}

func (r WorkloadRequest) IsGroupSelected(group string) bool {
	for _, g := range r.GroupBy {
		if g == group {
			return true
		}
	}
	return false
}

func (r WorkloadRequest) ToggleGroupLink(group string) string {
	if r.IsGroupSelected(group) {
		return r.RemoveGroupByLink(group)
	}
	return r.AddGroupByLink(group)
}

func (r WorkloadRequest) AddGroupByLink(groupBy string) string {
	r = r.Clone()
	r.GroupBy = append(r.GroupBy, groupBy)
	return r.Link()
}

func (r WorkloadRequest) RemoveGroupByLink(groupBy string) string {
	r = r.Clone()
	for i, g := range r.GroupBy {
		if g == groupBy {
			r.GroupBy = append(r.GroupBy[:i], r.GroupBy[i+1:]...)
			break
		}
	}
	if r.OderBy == groupBy {
		r.OderBy = ""
	}
	return r.Link()
}

func (r WorkloadRequest) SetTimeRangeLink(rangeStr string) string {
	r = r.Clone()
	r.Range = rangeStr
	r.Start = ""
	r.End = ""
	return r.Link()
}

func (r WorkloadRequest) Clone() WorkloadRequest {
	// Create a new instance of WorkloadAggRequest
	copyW := r

	// Deep copy the GroupBy slice
	copyW.GroupBy = make([]string, len(r.GroupBy))
	copy(copyW.GroupBy, r.GroupBy)

	return copyW
}

func (r WorkloadRequest) ToggleOrderLink(col string) string {
	if !queries.Contains(queries.AllowedSortBy(), col) {
		return ""
	}
	r = r.Clone()
	currentCol := strings.TrimSuffix(strings.TrimSuffix(r.OderBy, " desc"), " asc")
	if currentCol == col {
		if strings.HasSuffix(r.OderBy, " desc") {
			r.OderBy = currentCol + " asc"
		} else {
			r.OderBy = currentCol + " desc"
		}
	} else {
		r.OderBy = col
	}
	return r.Link()
}

func (r WorkloadRequest) IsOrderAsc(col string) bool {
	return strings.TrimSuffix(r.OderBy, " asc") == col
}

func (r WorkloadRequest) IsOrderDesc(col string) bool {
	return r.OderBy == col+" desc"
}

func (r WorkloadRequest) GroupedByMap() map[string]struct{} {
	result := make(map[string]struct{})
	for _, g := range r.GroupBy {
		result[g] = struct{}{}
	}
	return result
}

func (r WorkloadRequest) AvailableGroupBy() []string {
	result := make([]string, 0, len(queries.AllowedGroupBy()))
	groupedBy := r.GroupedByMap()
	for _, groupBy := range queries.AllowedGroupBy() {
		if _, ok := groupedBy[groupBy]; ok {
			continue
		}
		result = append(result, groupBy)
	}
	return result
}

func (r WorkloadRequest) Link() string {
	values := url.Values{
		"groupby": r.GroupBy,
	}
	if r.Start != "" {
		values.Set("start", r.Start)
	}
	if r.End != "" {
		values.Set("end", r.End)
	}
	if r.Range != "" {
		values.Set("range", r.Range)
	}
	if r.OderBy != "" {
		values.Set("orderby", r.OderBy)
	}
	u, _ := url.Parse("/workload")
	u.RawQuery = values.Encode()
	return u.String()
}

func (s *Srv) HandleWorkload(w http.ResponseWriter, r *http.Request) {
	workloadReq := UnmarshalWorkloadRequest(r.URL.Query())

	aggRequest, err := workloadReq.ToQuery()
	if err != nil {
		s.HttpError(w, err)
		return
	}

	aggData, err := s.queries.WorkloadAgg(r.Context(), aggRequest)
	if err != nil {
		s.HttpError(w, err)
		return
	}

	err = s.template.ExecuteTemplate(w, "index.html", &HomeTemplateData{
		Request:        workloadReq,
		AggData:        aggData,
		GroupByOptions: queries.AllowedGroupBy(),
		TimeRangeOptions: []TimeRangeOptions{
			{Label: "1h", Value: "1h"},
			{Label: "3h", Value: "3h"},
			{Label: "12h", Value: "12h"},
			{Label: "1d", Value: "24h"},
			{Label: "7d", Value: "168h"},
			{Label: "30d", Value: "720h"},
		},
	})
	if err != nil {
		s.HttpError(w, err)
		return
	}
}

func UnmarshalWorkloadRequest(v url.Values) WorkloadRequest {
	request := WorkloadRequest{
		GroupBy: uniq(v["groupby"]),
		OderBy:  v.Get("orderby"),
		Start:   v.Get("start"),
		End:     v.Get("end"),
		Range:   v.Get("range"),
	}
	if len(request.GroupBy) == 0 {
		request.GroupBy = queries.AllowedGroupBy()
	}
	return request
}

func formatData(v interface{}) string {
	switch v := v.(type) {
	case float64:
		return fmt.Sprintf("%.2f", v) // adjust precision as needed
	// add more cases here for other types if needed1
	default:
		return fmt.Sprint(v)
	}
}

func uniq(data []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, d := range data {
		if !seen[d] {
			result = append(result, d)
			seen[d] = true
		}
	}
	return result
}
