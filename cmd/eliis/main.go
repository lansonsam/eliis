// Package main is the eliis gateway entrypoint.
//
// M0.5 milestone: load config, start a Gin HTTP server with /health,
// support graceful shutdown on SIGINT/SIGTERM. Real protocol handlers
// will be wired in later milestones (see DESIGN.md §8).
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

	"github.com/gin-gonic/gin"

	"github.com/lansonsam/eliis/internal/core/config"
)

const (
	serviceName    = "eliis"
	serviceVersion = "0.1.0-dev"
	shutdownGrace  = 5 * time.Second

	// Compact, human-friendly timestamp layout (no nanos, no timezone).
	tsLayout = "2006-01-02 15:04:05"
)

func main() {
	configPath := flag.String("config", "configs/eliis.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config %q: %v\n", *configPath, err)
		os.Exit(1)
	}

	logger, err := newLogger(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	router := buildRouter()

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening",
			"service", serviceName,
			"version", serviceVersion,
			"addr", cfg.Server.Addr,
			"log_level", cfg.Log.Level,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("server crashed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped cleanly")
}

// newLogger builds a slog.Logger from config: level + format + compact time.
func newLogger(cfg config.Log) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, time.Now().Format(tsLayout))
			}
			return a
		},
	}

	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text", "":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("unknown log format %q (want text|json)", cfg.Format)
	}
	return slog.New(handler), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (want debug|info|warn|error)", s)
	}
}

func buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(accessLog())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": serviceName,
			"version": serviceVersion,
		})
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": serviceName,
			"version": serviceVersion,
			"docs":    "see DESIGN.md / STATUS.md",
		})
	})

	return r
}

// accessLog emits one slog.Debug record per request.
// At info level (default) it stays silent; flip log.level to debug to enable.
// slog itself short-circuits when the level is disabled, so cost is negligible.
func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !slog.Default().Enabled(c.Request.Context(), slog.LevelDebug) {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		slog.Debug("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
