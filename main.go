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

func main() {
	addr := flag.String("addr", "127.0.0.1:7000", "listen address")
	shell := flag.String("shell", "/bin/sh", "shell to spawn")
	flag.Parse()

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
		Handler: mux,
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
