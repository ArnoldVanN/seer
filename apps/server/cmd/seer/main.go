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

	"github.com/ArnoldVanN/seer/apps/server/internal/admin"
	"github.com/ArnoldVanN/seer/apps/server/internal/auth"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/db"
	"github.com/ArnoldVanN/seer/apps/server/internal/events"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/notify"
	"github.com/ArnoldVanN/seer/apps/server/internal/push"
	"github.com/ArnoldVanN/seer/apps/server/internal/rpc"
	"github.com/ArnoldVanN/seer/apps/server/internal/scheduler"
	"github.com/ArnoldVanN/seer/apps/server/internal/session"
	"github.com/ArnoldVanN/seer/apps/server/internal/sites"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1/adminv1connect"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/public/v1/publicv1connect"
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

	// Nil on an API-only replica: pings recorded there are queued for the
	// workers' dispatchers to pick up.
	var disp *notify.Dispatcher
	if worker {
		disp = notify.New(d, cfg.Alerting)
		w := scheduler.NewWorker(d, &scheduler.Recorder{DB: d, Notify: disp})
		g.Go(func() error { return w.Run(ctx) })
		g.Go(func() error { disp.Run(ctx); return nil })
		g.Go(func() error {
			jobs.RunPeriodic(ctx, d, cfg.Storage.Retention, scheduler.LiveTimeout)
			return nil
		})
	}

	if api {
		bus := events.NewBus(d.DSN)
		g.Go(func() error { bus.Run(ctx); return nil })
		siteStore := sites.New(d, cfg.UI)

		public := http.NewServeMux()
		path, h := publicv1connect.NewStatusServiceHandler(rpc.NewPublic(d, bus, siteStore))
		public.Handle(path, h)
		public.Handle(push.Path, push.New(d, &scheduler.Recorder{DB: d, Notify: disp}))
		serve(ctx, g, "public", cfg.Server.PublicPort, withCORS(rpc.WithSite(siteStore, public), cfg.Server.CORSOrigins))

		private := http.NewServeMux()
		var privateUp bool
		if cfg.Server.InternalJWTSecret == "" {
			log.Warn().Msg("server.internalJwtSecret is not set; private dashboards disabled")
		} else if err := checkSecret(cfg.Server.InternalJWTSecret); err != nil {
			return err
		} else {
			secret := cfg.Server.InternalJWTSecret
			sessions := session.New(d)
			private.Handle(privatev1connect.NewPrivateStatusServiceHandler(
				rpc.NewPrivate(d, bus, siteStore, sessions),
				connect.WithInterceptors(auth.NewInterceptor(secret, sessions)),
			))
			private.Handle(privatev1connect.NewSessionServiceHandler(
				&rpc.Sessions{Store: sessions},
				connect.WithInterceptors(auth.NewWebInterceptor(secret)),
			))
			privateUp = true
		}
		// The admin API isn't scoped to a site, so it skips WithSite.
		privateRoot := http.NewServeMux()
		privateRoot.Handle("/", rpc.WithSite(siteStore, private))
		if len(cfg.Server.AdminAPIKeys) > 0 {
			privateRoot.Handle(adminv1connect.NewAdminServiceHandler(
				&admin.Service{DB: d, Alerting: cfg.Alerting, Sites: siteStore},
				connect.WithInterceptors(auth.NewAdminInterceptor(cfg.Server.AdminAPIKeys)),
			))
			privateUp = true
		}
		if privateUp {
			serve(ctx, g, "private", cfg.Server.PrivatePort, privateRoot)
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

// minSecret matches the web app's check. Anyone with the secret can mint
// tokens for any group, so a short one is refused unless
// SEER_ALLOW_WEAK_SECRETS is set (development and demos).
const minSecret = 32

func checkSecret(secret string) error {
	if len(secret) >= minSecret {
		return nil
	}
	msg := fmt.Sprintf("server.internalJwtSecret is shorter than %d characters; generate one with `openssl rand -base64 32`", minSecret)
	if envBool("SEER_ALLOW_WEAK_SECRETS", false) {
		log.Warn().Msg(msg + " (allowed by SEER_ALLOW_WEAK_SECRETS)")
		return nil
	}
	return errors.New(msg + ", or set SEER_ALLOW_WEAK_SECRETS=true (development only)")
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
