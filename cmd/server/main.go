// Command server runs the fleet monitoring API: it loads the device
// roster from a CSV file and serves heartbeat/stats ingestion and
// reporting endpoints as described in openapi.json.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fleet-monitor/internal/api"
	"fleet-monitor/internal/device"
	"fleet-monitor/internal/telemetry"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	port := flag.Int("port", 6733, "port to listen on")
	devicesPath := flag.String("devices", "devices.csv", "path to the devices CSV file")
	flag.Parse()

	devices, err := device.LoadCSV(*devicesPath)
	if err != nil {
		slog.Error("failed to load devices", "path", *devicesPath, "error", err)
		os.Exit(1)
	}
	slog.Info("loaded devices", "count", len(devices), "path", *devicesPath)

	telemetryStore := telemetry.NewMemoryStore(devices)
	apiHandlers := api.NewHandlers(telemetryStore)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           api.NewRouter(apiHandlers),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", srv.Addr, "base_path", "/api/v1")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
}
