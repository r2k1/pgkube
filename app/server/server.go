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
	renderFunc func(w http.ResponseWriter, name string, data interface{})
	assetsPath string
}

func NewSrv(queries *queries.Queries, templatesPath string, assetsPath string, autoReload bool) *Srv {
	var renderer func(w http.ResponseWriter, name string, data interface{})
	if autoReload {
		renderer = func(w http.ResponseWriter, name string, data interface{}) {
			templates := MustTemplates(templatesPath)
			err := templates.ExecuteTemplate(w, name, data)
			if err != nil {
				HTTPError(w, err)
			}
		}
	} else {
		templates := MustTemplates(templatesPath)
		renderer = func(w http.ResponseWriter, name string, data interface{}) {
			err := templates.ExecuteTemplate(w, name, data)
			if err != nil {
				HTTPError(w, err)
			}
		}
	}
	return &Srv{
		renderFunc: renderer,
		queries:    queries,
		assetsPath: assetsPath,
	}
}

func MustTemplates(templatesPath string) *template.Template {
	funcMap := template.FuncMap{
		"byteCountSI": byteCountSI,
	}
	path := filepath.Join(templatesPath, "*.html")
	return template.Must(template.New("").Funcs(sprig.FuncMap()).Funcs(funcMap).ParseGlob(path))
}

func (s *Srv) Handler() http.Handler {
	fs := http.FileServer(http.Dir(s.assetsPath))

	mux := &http.ServeMux{}
	mux.Handle("/assets/", http.StripPrefix("/assets", fs))
	mux.Handle("/", http.RedirectHandler(DefaultRequest().Link(), http.StatusFound))
	mux.HandleFunc("/workload", s.HandleWorkload)
	return LoggingMiddleware(mux)
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

func HTTPError(w http.ResponseWriter, err error) {
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
