package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/r2k1/pgkube/app/queries"
)

type Srv struct {
	queries  *queries.Queries
	template *template.Template
}

func NewSrv(queries *queries.Queries) *Srv {
	funcMap := template.FuncMap{
		"ByteCountSI": ByteCountSI,
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))
	return &Srv{
		template: tmpl,
		queries:  queries,
	}
}

type HomeData struct {
	Request WorkloadRequest
	AggData *queries.WorkloadAggResult
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  []string
	Start   time.Time
	End     time.Time
}

func (r WorkloadRequest) Clone() WorkloadRequest {
	// Create a new instance of WorkloadRequest
	copyW := WorkloadRequest{
		Start: r.Start,
		End:   r.End,
	}

	// Deep copy the GroupBy slice
	copyW.GroupBy = make([]string, len(r.GroupBy))
	copy(copyW.GroupBy, r.GroupBy)

	// Deep copy the OderBy slice
	copyW.OderBy = make([]string, len(r.OderBy))
	copy(copyW.OderBy, r.OderBy)

	return copyW
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

func (r WorkloadRequest) AddGroupByLink(groupBy string) string {
	r = r.Clone()
	r.GroupBy = append(r.GroupBy, groupBy)
	return MarshalWorkloadRequest(r)
}

func (r WorkloadRequest) RemoveGroupByLink(groupBy string) string {
	r = r.Clone()
	for i, g := range r.GroupBy {
		if g == groupBy {
			r.GroupBy = append(r.GroupBy[:i], r.GroupBy[i+1:]...)
			break
		}
	}
	return MarshalWorkloadRequest(r)
}

func (s *Srv) HandleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	workloadReq, err := UnmarshalWorkloadRequest(r.URL.Query())
	if err != nil {
		s.HttpError(w, err)
		return
	}

	aggData, err := s.queries.WorkloadAgg(r.Context(), queries.WorkloadRequest(workloadReq))
	if err != nil {
		s.HttpError(w, err)
		return
	}

	err = s.template.ExecuteTemplate(w, "index.html", &HomeData{
		Request: workloadReq,
		AggData: aggData,
	})
	if err != nil {
		s.HttpError(w, err)
		return
	}
}

func UnmarshalWorkloadRequest(v url.Values) (WorkloadRequest, error) {
	result := WorkloadRequest{
		GroupBy: uniq(v["groupby"]),
		OderBy:  uniq(v["orderby"]),
	}
	if len(result.GroupBy) == 0 {
		result.GroupBy = queries.AllowedGroupBy()
	}

	return result, nil
}

func MarshalWorkloadRequest(request WorkloadRequest) string {
	values := url.Values{
		"groupby": request.GroupBy,
		"orderby": request.OderBy,
	}
	u, _ := url.Parse("/")
	u.RawQuery = values.Encode()
	return u.String()
}

func (s *Srv) Start(addr string) error {
	fs := http.FileServer(http.Dir("assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))
	http.HandleFunc("/", s.HandleHome)
	slog.Info("Starting server", "addr", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Srv) HttpError(w http.ResponseWriter, err error) {
	slog.Error("HTTP error", "error", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func ByteCountSI(b float64) string {
	const unit = 1000.0
	if b < unit {
		return fmt.Sprintf("%.2f B", b)
	}
	div, exp := unit, 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", b/div, "kMGTPE"[exp])
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
