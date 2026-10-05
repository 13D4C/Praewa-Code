package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"praewa-code/internal/agent"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	token := os.Getenv("PRAEWA_AGENT_PREVIEW_TOKEN")
	if token == "" {
		log.Fatal("Start this preview through the frontend development server")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	service := agent.New(func() string { return root })
	defer service.Close()
	server := &http.Server{Handler: service.Handler(token), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("PRAEWA_AGENT_PORT=%d", listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		cancel()
	}()
	go func() {
		<-ctx.Done()
		deadline, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = server.Shutdown(deadline)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
