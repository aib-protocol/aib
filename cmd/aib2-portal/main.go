package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
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
	addr := flag.String("addr", ":51200", "HTTPS listen address")
	certFile := flag.String("tls-cert", "", "TLS certificate file (auto-generate if empty)")
	keyFile := flag.String("tls-key", "", "TLS key file (auto-generate if empty)")
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

	var tlsConfig *tls.Config
	if *certFile == "" || *keyFile == "" {
		tlsConfig, err = selfSignedTLS()
		if err != nil {
			log.Fatalf("failed to generate self-signed cert: %v", err)
		}
	}

	httpServer := &http.Server{
		Addr:         *addr,
		Handler:      srv,
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("aib2-portal listening on https://84.247.155.30%s\n", *addr)
		if *certFile != "" && *keyFile != "" {
			err = httpServer.ListenAndServeTLS(*certFile, *keyFile)
		} else {
			ln, listenErr := tls.Listen("tcp", *addr, tlsConfig)
			if listenErr != nil {
				log.Fatalf("listen: %v", listenErr)
			}
			err = httpServer.Serve(ln)
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	fmt.Println("\nshutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	fmt.Println("stopped")
}

func selfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"AIB 2.0 Portal"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("84.247.155.30"), net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}, nil
}
