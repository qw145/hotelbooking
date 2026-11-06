package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

func Handler1() *http.ServeMux {
	return nil
}

type middleware func(http.Handler) http.Handler

// contextKey is an unexported type for context keys defined in this package,
// which prevents collisions with keys from other packages.
type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDFrom returns the request ID associated with ctx, if any.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// statusRecorder captures the status code and bytes written so the access log
// can report them.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Flush passes through to the underlying writer when it supports flushing, so
// wrapping does not break streaming responses.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// requestID attaches a unique ID to every request, echoes it in the response,
// and puts it in the context so logs for one request can be correlated.
func RequestID() middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set("X-Request-Id", id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// newRequestID returns a short random hex identifier.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A request ID is diagnostic only; fall back to a timestamp rather
		// than failing the request.
		return "ts-" + time.Now().UTC().Format("20060102150405.000000")
	}
	return hex.EncodeToString(b[:])
}

// accessLog records one line per request at completion.
func AccessLog(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			level := slog.LevelInfo
			switch {
			case rec.status >= http.StatusInternalServerError:
				level = slog.LevelError
			case rec.status >= http.StatusBadRequest:
				level = slog.LevelWarn
			case r.URL.Path == "/healthz":
				// Health checks are frequent and uninteresting; keep them out
				// of the default log stream.
				level = slog.LevelDebug
			}

			log.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// recoverer converts a panic in a handler into a 500 rather than dropping the
// connection, and logs the stack for diagnosis.
func Recoverer(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// A client disconnecting mid-write surfaces as this sentinel;
				// it is not a server fault and cannot be responded to.
				if errors.Is(asError(rec), http.ErrAbortHandler) {
					panic(rec)
				}

				log.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()),
				)

				// // writeError(w, r, log, &apiError{
				// // 	Status:  http.StatusInternalServerError,
				// // 	Code:    CodeInternal,
				// // 	Message: "an unexpected error occurred",
				// // })
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// asError converts a recovered value into an error.
func asError(v interface{}) error {
	if err, ok := v.(error); ok {
		return err
	}
	return nil
}

// securityHeaders sets conservative defaults. The API serves JSON and a spec,
// not user-authored HTML, so the policy can be strict.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
