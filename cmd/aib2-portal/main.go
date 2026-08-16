package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/data"
	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/render"
	"github.com/aib-protocol/aib/cmd/aib2-portal/internal/server"
)

func main() {
	addr := flag.String("addr", ":51200", "HTTP listen address")
	flag.Parse()

	// Load modules data
	modules, err := data.LoadModules(dataFS)
	if err != nil {
		log.Fatalf("failed to load modules: %v", err)
	}

	// Initialize template engine
	engine, err := render.New(templateFS)
	if err != nil {
		log.Fatalf("failed to init templates: %v", err)
	}

	// Create server
	srv := server.New(engine, modules, staticFS)

	httpServer := &http.Server{
		Addr:         *addr,
		Handler:      srv,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("aib2-portal listening on http://www.aib.one%s\n", *addr)
		err = httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}

	log.Println("server exited")
}
