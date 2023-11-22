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
		"byteCountSI": byteCountSI,
		"formatData":  formatData,
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))
	return &Srv{
		template: tmpl,
		queries:  queries,
	}
}

type HomeTemplateData struct {
	Request        WorkloadRequest
	AggData        *queries.WorkloadAggResult
	GroupByOptions []string
}

type WorkloadRequest struct {
	GroupBy []string
	OderBy  []string
	Start   time.Time
	End     time.Time
}

func DefaultRequest() WorkloadRequest {
	return WorkloadRequest{
		GroupBy: []string{"namespace", "controller_kind", "controller_name"},
		OderBy:  []string{"namespace"},
		Start:   time.Now().Add(-24 * time.Hour),
		End:     time.Now(),
	}
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

func (r WorkloadRequest) Clone() WorkloadRequest {
	// Create a new instance of WorkloadAggRequest
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
	if r.URL.RawQuery == "" {
		http.Redirect(w, r, MarshalWorkloadRequest(DefaultRequest()), http.StatusFound)
		return
	}

	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	workloadReq, err := UnmarshalWorkloadRequest(r.URL.Query())
	if err != nil {
		s.HttpError(w, err)
		return
	}

	aggData, err := s.queries.WorkloadAgg(r.Context(), queries.WorkloadAggRequest(workloadReq))
	if err != nil {
		s.HttpError(w, err)
		return
	}

	err = s.template.ExecuteTemplate(w, "index.html", &HomeTemplateData{
		Request:        workloadReq,
		AggData:        aggData,
		GroupByOptions: queries.AllowedGroupBy(),
	})
	if err != nil {
		s.HttpError(w, err)
		return
	}
}

func UnmarshalWorkloadRequest(v url.Values) (WorkloadRequest, error) {
	var err error
	result := WorkloadRequest{
		GroupBy: uniq(v["groupby"]),
		OderBy:  uniq(v["orderby"]),
	}
	startS := v.Get("start")
	endS := v.Get("end")

	switch {
	case startS == "" && endS == "":
		now := time.Now()
		result.Start = now.Add(-24 * time.Hour)
		result.End = now
	case startS != "" && endS != "":
		result.Start, err = time.Parse(time.RFC3339, startS)
		if err != nil {
			return WorkloadRequest{}, fmt.Errorf("invalid start: %w", err)
		}
		result.End, err = time.Parse(time.RFC3339, endS)
		if err != nil {
			return WorkloadRequest{}, fmt.Errorf("invalid end: %w", err)
		}
	default:
		return result, fmt.Errorf("invalid start/end")
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
		"start":   []string{request.Start.Format(time.RFC3339)},
		"end":     []string{request.End.Format(time.RFC3339)},
	}
	u, _ := url.Parse("/")
	u.RawQuery = values.Encode()
	return u.String()
}

func (s *Srv) Start(addr string) error {
	mux := &http.ServeMux{}
	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", s.HandleHome)
	slog.Info("Starting server", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := srv.ListenAndServe()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

func (s *Srv) HttpError(w http.ResponseWriter, err error) {
	slog.Error("HTTP error", "error", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func byteCountSI(b float64) string {
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
