package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/samber/lo"

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

func (t *TimeRangeOptions) StartDate() string {
	start, _, _ := rangeToStartEnd(t.Value)
	if start.IsZero() {
		return ""
	}
	return start.Format(time.RFC3339)
}

func (t *TimeRangeOptions) EndDate() string {
	_, end, _ := rangeToStartEnd(t.Value)
	if end.IsZero() {
		return ""
	}
	return end.Format(time.RFC3339)
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  string
	Range   string
	Start   time.Time
	End     time.Time
}

func DefaultRequest() WorkloadRequest {
	return WorkloadRequest{
		GroupBy: []string{"namespace", "controller_kind", "controller_name"},
		OderBy:  "namespace",
		Range:   "168h",
	}
}

func (r WorkloadRequest) ToQuery() (queries.WorkloadAggRequest, error) {
	if r.Range != "" && (!r.Start.IsZero() || !r.End.IsZero()) {
		return queries.WorkloadAggRequest{}, fmt.Errorf("range and start/end are mutually exclusive")
	}
	var start, end time.Time

	if r.Start.IsZero() || r.End.IsZero() {
		var err error
		start, end, err = rangeToStartEnd(r.Range)
		if err != nil {
			return queries.WorkloadAggRequest{}, err
		}
	} else {
		start = r.Start
		end = r.End
	}
	return queries.WorkloadAggRequest{
		GroupBy: r.GroupBy,
		OderBy:  r.OderBy,
		Start:   start,
		End:     end,
	}, nil
}

func rangeToStartEnd(rangeStr string) (time.Time, time.Time, error) {
	if rangeStr == "" {
		return time.Time{}, time.Time{}, nil
	}
	duration, err := time.ParseDuration(rangeStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid range: %w", err)
	}
	end := time.Now().UTC().Truncate(time.Hour)
	start := end.Add(-duration)
	return start, end, nil
}

func (r WorkloadRequest) IsGroupSelected(group string) bool {
	for _, g := range r.GroupBy {
		if g == group {
			return true
		}
	}
	return false
}

func (r WorkloadRequest) LinkToggleGroup(group string) string {
	if r.IsGroupSelected(group) {
		return r.LinkRemoveGroup(group)
	}
	return r.LinkAddGroup(group)
}

func (r WorkloadRequest) LinkAddGroup(groupBy string) string {
	r = r.Clone()
	r.GroupBy = append(r.GroupBy, groupBy)
	return r.Link()
}

func (r WorkloadRequest) LinkRemoveGroup(groupBy string) string {
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

func (r WorkloadRequest) LinkSetRange(rangeValue string) string {
	r = r.Clone()
	r.Range = rangeValue
	return r.Link()
}

func (r WorkloadRequest) LinkPrev() string {
	start := r.StartDate()
	end := r.EndDate()
	r = r.Clone()
	dur := -end.Sub(start)
	r.Start = start.Add(dur)
	r.End = end.Add(dur)
	r.Range = ""
	return r.Link()
}

func (r WorkloadRequest) LinkNext() string {
	start := r.StartDate()
	end := r.EndDate()
	r = r.Clone()
	dur := end.Sub(start)
	r.Start = start.Add(dur)
	r.End = end.Add(dur)
	r.Range = ""
	return r.Link()
}

func (r WorkloadRequest) LinkToggleOrder(col string) string {
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

func (r WorkloadRequest) Clone() WorkloadRequest {
	// Create a new instance of WorkloadAggRequest
	copyW := r

	// Deep copy the GroupBy slice
	copyW.GroupBy = make([]string, len(r.GroupBy))
	copy(copyW.GroupBy, r.GroupBy)

	return copyW
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

	if r.Range != "" {
		values.Set("range", r.Range)
	} else {
		if !r.Start.IsZero() {
			values.Set("start", r.StartValue())
		}
		if !r.End.IsZero() {
			values.Set("end", r.EndValue())
		}
	}
	if r.OderBy != "" {
		values.Set("orderby", r.OderBy)
	}
	u, _ := url.Parse("/workload")
	u.RawQuery = values.Encode()
	return u.String()
}

func (r WorkloadRequest) StartDate() time.Time {
	if r.Range != "" {
		start, _, _ := rangeToStartEnd(r.Range)
		return start
	}
	return r.Start
}

func (r WorkloadRequest) EndDate() time.Time {
	if r.Range != "" {
		_, end, _ := rangeToStartEnd(r.Range)
		return end
	}
	return r.End
}

func (r WorkloadRequest) StartValue() string {
	return timeToString(r.StartDate())
}

func (r WorkloadRequest) EndValue() string {
	return timeToString(r.EndDate())
}

func timeToString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (s *Srv) HandleWorkload(w http.ResponseWriter, r *http.Request) {
	workloadReq := UnmarshalWorkloadRequest(r.URL.Query())

	aggRequest, err := workloadReq.ToQuery()
	if err != nil {
		HTTPError(w, err)
		return
	}

	aggData, err := s.queries.WorkloadAgg(r.Context(), aggRequest)
	if err != nil {
		HTTPError(w, err)
		return
	}

	w.Header().Set("HX-Replace-Url", workloadReq.Link())

	s.renderFunc(w, "index.gohtml", &HomeTemplateData{
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
}

func UnmarshalWorkloadRequest(v url.Values) WorkloadRequest {
	result := WorkloadRequest{}
	result.Range = v.Get("range")
	if result.Range == "undefined" {
		result.Range = ""
	}
	if result.Range == "" {
		result.Start, _ = time.Parse(time.RFC3339, v.Get("start"))
		result.End, _ = time.Parse(time.RFC3339, v.Get("end"))
	}

	result.GroupBy = lo.Filter(v["groupby"], func(item string, index int) bool {
		return item != "" && item != "undefined"
	})
	result.GroupBy = lo.Uniq(result.GroupBy)
	result.OderBy = v.Get("orderby")
	if len(result.GroupBy) == 0 {
		result.GroupBy = queries.AllowedGroupBy()
	}
	return result
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
