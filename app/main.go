package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lmittmann/tint"
	"golang.org/x/term"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	//_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/r2k1/pgkube/app/queries"
	"github.com/r2k1/pgkube/app/scraper"
	"github.com/r2k1/pgkube/app/server"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	KubeConfig  string `env:"KUBECONFIG,required,expand" envDefault:"${HOME}/.kube/config"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"INFO"`
	Addr        string `env:"ADDR" envDefault:":8080"`
}

func (c *Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	err := Execute(context.Background())
	if err != nil {
		slog.Error("Exiting", "error", err)
		os.Exit(1)
	}
	slog.Info("Exiting")
	os.Exit(0)
}

func Execute(ctx context.Context) error {
	_ = godotenv.Load(".env")

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	w := os.Stderr
	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			NoColor: !term.IsTerminal(int(w.Fd())),
			Level:   cfg.SlogLevel(),
		}),
	)
	slog.SetDefault(logger)

	if err := Migrate(cfg.DatabaseURL); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}
	defer pool.Close()
	queries := queries.New(pool)

	clientset, err := K8sClientset(cfg)
	if err != nil {
		return err
	}

	cache := scraper.NewCache()

	err = scraper.StartScraper(ctx, queries, clientset, time.Minute, cache)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		err := server.NewSrv(queries, "templates", "assets").Start(cfg.Addr)
		if err != nil {
			slog.Error("server error", "error", err)
		}
		cancel()
	}()
	<-ctx.Done()
	return fmt.Errorf("context done: %w", ctx.Err())
}

func Migrate(databaseURL string) error {
	if strings.HasPrefix(databaseURL, "postgres://") {
		databaseURL = strings.TrimPrefix(databaseURL, "postgres")
		databaseURL = "pgx5" + databaseURL
	} else {
		return fmt.Errorf("unsupported database url")
	}
	m, err := migrate.New(
		"file://migrations",
		databaseURL)
	if err != nil {
		return fmt.Errorf("unable to create migration: %w", err)
	}
	defer m.Close()

	err = m.Up()

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("DB is up to date")
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to migrate DB: %w", err)
	}
	slog.Info("DB migrated")
	return nil
}

func K8sClientset(cfg Config) (*kubernetes.Clientset, error) {
	clusterConfig, err := k8sConfig(cfg)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s clientset: %w", err)
	}
	return clientset, nil
}

func k8sConfig(cfg Config) (*rest.Config, error) {
	config, inClusterErr := rest.InClusterConfig()
	if inClusterErr == nil {
		return config, nil
	}
	home := homedir.HomeDir()
	if home == "" {
		return nil, errors.New("home directory not found")
	}
	config, outClusterErr := clientcmd.BuildConfigFromFlags("", cfg.KubeConfig)
	if outClusterErr == nil {
		return config, nil
	}
	return nil, errors.Join(inClusterErr, outClusterErr)
}
