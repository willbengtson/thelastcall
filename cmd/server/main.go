// Command server runs The Last Call: the JSON booking API and the site itself.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/willbengtson/thelastcall/internal/booking"
	"github.com/willbengtson/thelastcall/internal/config"
	"github.com/willbengtson/thelastcall/internal/httpapi"
	"github.com/willbengtson/thelastcall/internal/store"
	"github.com/willbengtson/thelastcall/internal/webui"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	loc, err := cfg.Location()
	if err != nil {
		return err
	}

	bookingStore, err := openStore(ctx, log, cfg)
	if err != nil {
		return err
	}

	sched := booking.DefaultSchedule
	bookings := booking.NewService(bookingStore, booking.Options{
		Schedule: sched,
		Bays:     cfg.Bays,
		MaxParty: cfg.MaxParty,
		Window:   cfg.BookingWindow,
		Lead:     cfg.BookingLead,
		Now:      func() time.Time { return time.Now().In(loc) },
	})

	ui := webui.FS()
	if ui == nil {
		log.Warn("frontend not built into this binary; serving API only",
			"hint", "run `make build-web`, or use the Vite dev server")
	}

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewServer(httpapi.Options{
			Log:            log,
			Bookings:       bookings,
			House:          httpapi.HouseFromSchedule("The Last Call", cfg.Timezone, sched, cfg.Bays, cfg.MaxParty),
			UI:             ui,
			AllowedOrigins: cfg.AllowedOrigins,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("the last call is open", "addr", cfg.Addr, "store", cfg.Store, "bays", cfg.Bays, "timezone", cfg.Timezone)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		log.Info("last call; draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	return nil
}

// openStore builds the configured persistence backend.
func openStore(ctx context.Context, log *slog.Logger, cfg config.Config) (booking.Store, error) {
	if cfg.Store == "memory" {
		log.Warn("using the in-memory store; reservations vanish on restart")
		return store.NewMemory(), nil
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	var opts []func(*dynamodb.Options)
	if cfg.DynamoEndpoint != "" {
		endpoint := cfg.DynamoEndpoint
		opts = append(opts, func(o *dynamodb.Options) { o.BaseEndpoint = &endpoint })
		log.Info("pointing DynamoDB at a local endpoint", "endpoint", endpoint)
	}

	dyn := store.NewDynamo(dynamodb.NewFromConfig(awsCfg, opts...), cfg.DynamoTable)
	if cfg.AutoCreateTable {
		if err := dyn.EnsureTable(ctx); err != nil {
			return nil, fmt.Errorf("preparing table: %w", err)
		}
		log.Info("table ready", "table", cfg.DynamoTable)
	}
	return dyn, nil
}
