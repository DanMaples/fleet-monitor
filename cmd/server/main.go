// Command server runs the fleet monitoring API: it loads the device
// roster from a CSV file and serves heartbeat/stats ingestion and
// reporting endpoints as described in openapi.json.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"fleet-monitor/internal/api"
	"fleet-monitor/internal/device"
	"fleet-monitor/internal/telemetry"
)

func main() {
	port := flag.Int("port", 6733, "port to listen on")
	devicesPath := flag.String("devices", "devices.csv", "path to the devices CSV file")
	flag.Parse()

	devices, err := device.LoadCSV(*devicesPath)
	if err != nil {
		log.Fatalf("failed to load devices from %s: %v", *devicesPath, err)
	}
	log.Printf("loaded %d device(s) from %s", len(devices), *devicesPath)

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
		log.Printf("listening on %s (base path /api/v1)", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
}
