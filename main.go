// Package puretty provides a single-binary web terminal for local
// machine access.
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

const errorPage = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>puretty — token required</title></head>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">
<h1>puretty</h1>
<p>A token is required to access this terminal.</p>
<p>Append <code>?token=…</code> to the URL — the token is printed to stdout when the server starts.</p>
</body>
</html>`

func main() {
	addr := flag.String("addr", "127.0.0.1:7000", "listen address")
	shell := flag.String("shell", "/bin/sh", "shell to spawn")
	tokenTTL := flag.Duration("token-ttl", 1*time.Hour, "token rotation interval (0 disables)")
	flag.Parse()

	tm := NewTokenManager(*tokenTTL)

	session, err := NewSession(*shell)
	if err != nil {
		log.Fatalf("failed to create session: %v", err)
	}

	h := &handler{session: session}

	mux := http.NewServeMux()
	mux.HandleFunc("/input", h.handleInput)
	mux.HandleFunc("/output", h.handleOutput)
	mux.HandleFunc("/resize", h.handleResize)

	// Serve embedded web files at /.
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		mux.Handle("/", http.FileServer(http.FS(sub)))
	} else {
		log.Printf("warning: embedded web fs not available: %v", err)
	}

	server := &http.Server{
		Addr:    *addr,
		Handler: tokenMiddleware(tm, loggingMiddleware(mux)),
	}

	// Graceful shutdown on either signal or shell exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			log.Printf("received %v, shutting down", sig)
		case <-session.Done():
			log.Print("shell exited, shutting down")
		}

		_ = session.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s (shell: %s)", *addr, *shell)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// loggingMiddleware logs every HTTP request: method, path, remote addr, status.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		log.Printf("%s %s %s %d", r.Method, r.URL.RequestURI(), r.RemoteAddr, lrw.status)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

// tokenMiddleware enforces token access. If token auth is disabled, it
// passes through. Valid tokens from query params are set as a cookie so
// subsequent same-origin requests (static files, XHR) work transparently.
func tokenMiddleware(tm *TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			if cookie, err := r.Cookie("puretty_token"); err == nil {
				token = cookie.Value
			}
		}

		if !tm.Validate(token) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errorPage))
			return
		}

		// Token came from query param and was valid — set cookie for subsequent requests.
		if token != "" && r.URL.Query().Get("token") != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "puretty_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		next.ServeHTTP(w, r)
	})
}
