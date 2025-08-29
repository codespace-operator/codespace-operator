package server

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	cblog "github.com/charmbracelet/log"
)

// Shared app logger for the web server package.
var logger = cblog.NewWithOptions(os.Stderr, cblog.Options{
	ReportTimestamp: true,
	TimeFormat:      time.RFC3339,
	ReportCaller:    false, // flip to true if you want file:line
})

func GetLogger() *cblog.Logger {
	return logger
}

// Call this once in main after reading config.
func configureLogger(logLevel string) {
	if logLevel == "debug" {
		logger.SetLevel(cblog.DebugLevel)
		logger.SetReportCaller(true)
	} else if logLevel == "info" {
		logger.SetLevel(cblog.InfoLevel)
	} else if logLevel == "warn" {
		logger.SetLevel(cblog.WarnLevel)
	} else {
		logger.SetLevel(cblog.ErrorLevel)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// attach / propagate a request id
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = randB64(6)
			w.Header().Set("X-Request-Id", reqID)
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)

		// include claims (when present for /api/*), request id (if any), and client ip
		user := "-"
		if cl := fromContext(r); cl != nil && cl.Sub != "" {
			user = cl.Sub
		}
		if reqID == "" {
			reqID = "-"
		}
		ip := clientIP(r)

		logger.Info("http",
			"method", r.Method,
			"path", r.URL.RequestURI(),
			"status", rw.statusCode,
			"bytes", rw.bytes,
			"dur", time.Since(start),
			"ip", ip,
			"ua", r.UserAgent(),
			"req_id", reqID,
			"user", user,
		)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" {
		return host
	}
	return r.RemoteAddr
}
