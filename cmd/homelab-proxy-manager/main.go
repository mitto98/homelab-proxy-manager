// Command homelab-proxy-manager runs the Panel.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/api"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
	"github.com/mitto98/homelab-proxy-manager/internal/version"
	"github.com/mitto98/homelab-proxy-manager/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := env("HPM_DATA_DIR", "./data")
	listen := env("HPM_LISTEN", ":81")

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	handler, err := api.New(st, web.Dist())
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	slog.Info("starting", "version", version.Version, "listen", listen, "data", dataDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
