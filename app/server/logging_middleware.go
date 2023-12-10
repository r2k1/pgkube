package server

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter captures HTTP response details
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

// NewResponseWriter initializes a new responseWriter
func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	// Default status code to 200 for cases where WriteHeader is not explicitly called
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

// WriteHeader captures the status code and calls the original WriteHeader method
func (rw *responseWriter) WriteHeader(statusCode int) {
	if !rw.wroteHeader {
		rw.statusCode = statusCode
	}
	rw.ResponseWriter.WriteHeader(statusCode)
	rw.wroteHeader = true
}

// Write captures the number of bytes written and calls the original Write method
func (rw *responseWriter) Write(bytes []byte) (int, error) {
	count, err := rw.ResponseWriter.Write(bytes)
	rw.bytesWritten += count
	// nolint: wrapcheck
	return count, err
}

// LoggingMiddleware logs information about each HTTP request including status code
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		customWriter := NewResponseWriter(w)

		// Call the handler with our custom ResponseWriter
		next.ServeHTTP(customWriter, r)

		// Calculate the duration
		duration := time.Since(start)

		// Log using structured logging
		slog.Info("HTTP",
			slog.String("method", r.Method),
			slog.String("url", r.URL.String()),
			slog.Int("status", customWriter.statusCode),
			slog.Int("responseSize", customWriter.bytesWritten),
			slog.Duration("duration", duration),
		)
	})
}
