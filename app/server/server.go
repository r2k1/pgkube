package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

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

func (s *Srv) HandleHome(w http.ResponseWriter, r *http.Request) {
	usage, err := s.queries.WorkloadAgg(r.Context(), queries.WorkloadRequest{
		GroupBy: nil,
		OderBy:  []string{"namespace"},
	})
	if err != nil {
		s.HttpError(w, err)
		return
	}
	err = s.template.ExecuteTemplate(w, "index.html", usage)
	if err != nil {
		s.HttpError(w, err)
		return
	}
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
