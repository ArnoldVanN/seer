// Command seer runs the status checker and API.
//
//	seer all       API + worker in one process (homelab, compose)
//	seer api       public + private Connect APIs
//	seer worker    runs checks for the endpoints it owns
//	seer migrate   applies database migrations and exits
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	connectcors "connectrpc.com/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"connectrpc.com/connect"

	"github.com/seer-status/seer/apps/server/internal/auth"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
	"github.com/seer-status/seer/apps/server/internal/events"
	"github.com/seer-status/seer/apps/server/internal/jobs"
	"github.com/seer-status/seer/apps/server/internal/notify"
	"github.com/seer-status/seer/apps/server/internal/rpc"
	"github.com/seer-status/seer/apps/server/internal/scheduler"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/public/v1/publicv1connect"
)

var version = "dev"

func main() {
	setupLogging()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	configPath := fs.String("config", envOr("SEER_CONFIG", "seer.yaml"), "path to the config file")
	migrate := fs.Bool("migrate", envBool("SEER_MIGRATE_ON_START", true), "apply migrations on startup")
	fs.Parse(os.Args[2:]) //nolint:errcheck // ExitOnError

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "all":
		err = run(ctx, *configPath, *migrate, true, true)
	case "api":
		err = run(ctx, *configPath, *migrate, true, false)
	case "worker":
		err = run(ctx, *configPath, *migrate, false, true)
	case "migrate":
		err = runMigrate(ctx, *configPath)
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal().Err(err).Msg(cmd + " failed")
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: seer <all|api|worker|migrate|version> [-config seer.yaml] [-migrate=false]")
}

func setupLogging() {
	zerolog.TimeFieldFormat = time.RFC3339
	if lvl, err := zerolog.ParseLevel(envOr("SEER_LOG_LEVEL", "info")); err == nil {
		zerolog.SetGlobalLevel(lvl)
	}
	if os.Getenv("SEER_LOG_FORMAT") == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// runMigrate only needs a DSN, so the config file is optional: the Helm
// pre-upgrade Job runs before the new ConfigMap exists.
func runMigrate(ctx context.Context, configPath string) error {
	dsn := os.Getenv("SEER_DB_DSN")
	if cfg, err := config.Load(configPath); err == nil {
		dsn = cfg.Storage.DSN
	} else if !errors.Is(err, os.ErrNotExist) || dsn == "" {
		return err
	}
	d, err := db.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Migrate(ctx)
}

func run(ctx context.Context, configPath string, migrate, api, worker bool) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	d, err := db.Open(ctx, cfg.Storage.DSN)
	if err != nil {
		return err
	}
	defer d.Close()

	if migrate {
		if err := d.Migrate(ctx); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	host, _ := os.Hostname()
	generation, _ := strconv.ParseInt(os.Getenv("SEER_CONFIG_GENERATION"), 10, 64)
	if err := jobs.SyncConfig(ctx, d, cfg, generation, host); err != nil {
		return fmt.Errorf("config sync: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	ops := http.NewServeMux()
	ops.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	ops.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.SQL.PingContext(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	ops.Handle("/metrics", promhttp.Handler())
	serve(ctx, g, "ops", cfg.Server.OpsPort, ops)

	if worker {
		w := scheduler.NewWorker(d, &scheduler.Recorder{DB: d, Notify: notify.NewDispatcher()})
		g.Go(func() error { return w.Run(ctx) })
		g.Go(func() error {
			jobs.RunPeriodic(ctx, d, cfg.Storage.Retention, scheduler.LiveTimeout)
			return nil
		})
	}

	if api {
		bus := events.NewBus(d.DSN)
		g.Go(func() error { bus.Run(ctx); return nil })

		public := http.NewServeMux()
		path, h := publicv1connect.NewStatusServiceHandler(rpc.NewPublic(d, bus, cfg))
		public.Handle(path, h)
		serve(ctx, g, "public", cfg.Server.PublicPort, withCORS(rpc.WithSite(cfg, public), cfg.Server.CORSOrigins))

		if cfg.Server.InternalJWTSecret == "" {
			log.Warn().Msg("server.internalJwtSecret is not set; private API disabled")
		} else {
			private := http.NewServeMux()
			path, h := privatev1connect.NewPrivateStatusServiceHandler(
				rpc.NewPrivate(d, bus, cfg),
				connect.WithInterceptors(auth.NewInterceptor(cfg.Server.InternalJWTSecret)),
			)
			private.Handle(path, h)
			serve(ctx, g, "private", cfg.Server.PrivatePort, rpc.WithSite(cfg, private))
		}
	}

	log.Info().Str("version", version).Bool("api", api).Bool("worker", worker).Msg("seer started")
	return g.Wait()
}

// serve runs srv until ctx is done, then drains it. Plaintext HTTP/2 is on so
// connect-node clients can use h2c inside the cluster.
func serve(ctx context.Context, g *errgroup.Group, name string, port int, h http.Handler) {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           h,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	g.Go(func() error {
		log.Info().Str("server", name).Str("addr", srv.Addr).Msg("listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("%s server: %w", name, err)
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		// Streams never finish on their own; give unary calls a moment, then cut.
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			srv.Close()
		}
		return nil
	})
}

func withCORS(h http.Handler, origins []string) http.Handler {
	opts := cors.Options{
		AllowedMethods: connectcors.AllowedMethods(),
		AllowedHeaders: connectcors.AllowedHeaders(),
		ExposedHeaders: connectcors.ExposedHeaders(),
		MaxAge:         7200,
	}
	if len(origins) == 0 {
		opts.AllowedOrigins = []string{"*"}
	} else {
		opts.AllowedOrigins = origins
	}
	return cors.New(opts).Handler(h)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(k)); err == nil {
		return v
	}
	return def
}
