package server

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/r2k1/pgkube/app/queries"
)

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
	Cols   []string
	OderBy string
	Range  string
	Start  time.Time
	End    time.Time
}

func DefaultRequest() WorkloadRequest {
	return WorkloadRequest{
		Cols:   []string{"namespace", "controller_kind", "controller_name", "request_cpu_cores", "used_cpu_cores", "request_memory_bytes", "used_memory_bytes", "total_cost"},
		OderBy: "namespace",
		Range:  "168h",
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
		Cols:    r.Cols,
		OrderBy: r.OderBy,
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
	now := time.Now().UTC()
	end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	start := end.Add(-duration)
	return start, end, nil
}

func (r WorkloadRequest) IsColSelected(col string) bool {
	for _, g := range r.Cols {
		if g == col {
			return true
		}
	}
	return false
}

func (r WorkloadRequest) LinkRange(rangeValue string) string {
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

func (r WorkloadRequest) Duration() string {
	start := r.StartDate()
	end := r.EndDate()
	hours := int(end.Sub(start).Hours())
	return fmt.Sprintf("%dh", hours)
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
	if !queries.Contains(queries.Cols(), col) {
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

func (r WorkloadRequest) LinkToggleCol(col string) string {
	r = r.Clone()
	if r.IsColSelected(col) {
		r.Cols = lo.Filter(r.Cols, func(item string, _ int) bool {
			return item != col
		})
	} else {
		r.Cols = append(r.Cols, col)
	}
	return r.Link()
}

func (r WorkloadRequest) Clone() WorkloadRequest {
	// Create a new instance of WorkloadAggRequest
	copyW := r

	// Deep copy the GroupBy slice
	copyW.Cols = make([]string, len(r.Cols))
	copy(copyW.Cols, r.Cols)

	return copyW
}

func (r WorkloadRequest) IsOrderAsc(col string) bool {
	return strings.TrimSuffix(r.OderBy, " asc") == col
}

func (r WorkloadRequest) IsOrderDesc(col string) bool {
	return r.OderBy == col+" desc"
}

func (r WorkloadRequest) Link() string {
	values := r.urlValues()
	u, _ := url.Parse("/workload")
	u.RawQuery = values.Encode()
	return u.String()
}

func (r WorkloadRequest) urlValues() url.Values {
	values := url.Values{
		"col": r.Cols,
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
	return values
}

func (r WorkloadRequest) LinkCSV() string {
	values := r.urlValues()
	u, _ := url.Parse("/workload.csv")
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

func (r WorkloadRequest) Labels() []string {
	return lo.Filter(r.Cols, func(item string, _ int) bool {
		return strings.HasPrefix(item, "label_")
	})
}

func timeToString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (s *Srv) HandleWorkload(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) == 0 {
		http.Redirect(w, r, DefaultRequest().Link(), http.StatusFound)
		return
	}
	workloadReq, aggData, err := s.fetchWorkloadData(r)
	if err != nil {
		HTTPError(w, err)
		return
	}

	w.Header().Set("HX-Replace-Url", workloadReq.Link())

	data := struct {
		Request          WorkloadRequest
		AggData          *queries.WorkloadAggResult
		TimeRangeOptions []TimeRangeOptions
		Cols             []string
	}{
		Request: workloadReq,
		AggData: aggData,
		Cols:    queries.Cols(),
		TimeRangeOptions: []TimeRangeOptions{
			{Label: "Last 1h", Value: "1h"},
			{Label: "3h", Value: "3h"},
			{Label: "12h", Value: "12h"},
			{Label: "1d", Value: "24h"},
			{Label: "7d", Value: "168h"},
			{Label: "30d", Value: "720h"},
		},
	}

	s.renderFunc(w, "index.gohtml", data)
}

func (s *Srv) HandleWorkloadCSV(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) == 0 {
		http.Redirect(w, r, DefaultRequest().Link(), http.StatusFound)
		return
	}

	_, aggData, err := s.fetchWorkloadData(r)
	if err != nil {
		HTTPError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=pgkube.csv")
	err = writeAsCSV(w, aggData)
	if err != nil {
		HTTPError(w, err)
		return
	}
}

func (s *Srv) fetchWorkloadData(r *http.Request) (WorkloadRequest, *queries.WorkloadAggResult, error) {
	workloadReq := UnmarshalWorkloadRequest(r.URL.Query())

	aggRequest, err := workloadReq.ToQuery()
	if err != nil {
		return workloadReq, nil, err
	}
	aggData, err := s.queries.WorkloadAgg(r.Context(), aggRequest)

	if err != nil {
		return workloadReq, nil, err
	}
	return workloadReq, aggData, nil

}

func writeAsCSV(w http.ResponseWriter, aggData *queries.WorkloadAggResult) error {
	csvWriter := csv.NewWriter(w)

	if err := csvWriter.Write(aggData.Columns); err != nil {
		return err
	}
	for _, row := range aggData.Rows {
		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return nil
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
		if result.Start.IsZero() || result.End.IsZero() {
			result.Start = time.Time{}
			result.End = time.Time{}
			result.Range = "24h"
		}
	}
	result.Cols = lo.Filter(v["col"], func(item string, index int) bool {
		return item != "" && item != "undefined"
	})
	result.Cols = lo.Uniq(result.Cols)
	result.OderBy = v.Get("orderby")
	result.Start = TruncateHour(result.Start)
	result.End = TruncateHour(result.End)
	return result
}

func TruncateHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}
