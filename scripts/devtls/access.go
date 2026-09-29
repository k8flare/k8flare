package main

import (
	"log"
	"net/http"
	"time"
)

type countingWriter struct {
	http.ResponseWriter
	status   int
	bytes    int64
	lastByte time.Time
}

func (w *countingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	if n > 0 {
		w.lastByte = time.Now()
	}
	return n, err
}

func (w *countingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("15:04:05.000")
}

func accessLog(next http.Handler, longerThan time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		cw := &countingWriter{ResponseWriter: w}
		next.ServeHTTP(cw, r)
		end := time.Now()
		if r.URL.Query().Get("watch") != "true" && end.Sub(start) < longerThan {
			return
		}
		log.Printf("devtls: access %s %s status=%d start=%s end=%s bytes=%d last_byte=%s",
			r.Method, r.URL.RequestURI(), cw.status, stamp(start), stamp(end), cw.bytes, stamp(cw.lastByte))
	})
}
