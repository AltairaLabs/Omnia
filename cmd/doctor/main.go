package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/altairalabs/omnia/internal/doctor"
	"github.com/altairalabs/omnia/pkg/logging"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	log, sync, err := logging.NewLogger()
	if err != nil {
		os.Exit(1)
	}
	defer sync()

	runner := doctor.NewRunner()

	srv := doctor.NewServer(runner, *addr, log)
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("doctor starting", "addr", *addr)
		if srvErr := httpSrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			log.Error(srvErr, "server failed")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "shutdown failed")
	}
}
