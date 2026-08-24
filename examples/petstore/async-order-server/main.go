// Copyright 2026 novassist.ai. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// AsyncAPI place-order / confirm-order adapter. Places orders on Petstore 3
// (local Docker or hosted) via POST /store/order.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "localhost:8091", "listen address")
	petstore := flag.String("petstore", "local", "Petstore 3 target: local (Docker on :8090) or hosted (petstore3.swagger.io)")
	petstoreURL := flag.String("petstore-url", "", "override Petstore 3 OpenAPI origin")
	flag.Parse()

	petstoreBase, err := resolvePetstoreBase(*petstore, *petstoreURL)
	if err != nil {
		log.Fatal(err)
	}

	s := newOrderServer([]string{petstoreBase})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /place-order", s.handlePlaceOrder)
	mux.HandleFunc("GET /confirm-order", s.handleConfirmOrder)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Printf("async order adapter http://%s  POST /place-order  GET /confirm-order  petstore %s", *addr, petstoreBase)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
