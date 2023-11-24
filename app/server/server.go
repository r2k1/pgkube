package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Masterminds/sprig/v3"

	"github.com/r2k1/pgkube/app/queries"
)

type Srv struct {
	queries    *queries.Queries
	template   *template.Template
	assetsPath string
}

func NewSrv(queries *queries.Queries, templatesPath string, assetsPath string) *Srv {
	funcMap := template.FuncMap{
		"byteCountSI": byteCountSI,
		"formatData":  formatData,
	}
	path := filepath.Join(templatesPath, "*.html")
	tmpl := template.Must(template.New("").Funcs(sprig.FuncMap()).Funcs(funcMap).ParseGlob(path))
	return &Srv{
		template:   tmpl,
		queries:    queries,
		assetsPath: assetsPath,
	}
}

func (s *Srv) Handler() http.Handler {
	fs := http.FileServer(http.Dir(s.assetsPath))

	mux := &http.ServeMux{}
	mux.Handle("/", http.RedirectHandler(DefaultRequest().Link(), http.StatusFound))
	mux.HandleFunc("/workload", s.HandleWorkload)
	mux.Handle("/assets/*", http.StripPrefix("/assets", fs))
	return mux
}

func (s *Srv) Start(addr string) error {
	slog.Info("Starting server", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
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
