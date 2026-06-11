// Package main is the eliis gateway entrypoint.
//
// M1 milestone: load config, start a Gin HTTP server with /health, support
// Anthropic Messages ingress backed by an OpenAI-compatible upstream, and
// gracefully shutdown on SIGINT/SIGTERM.
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

	backendopenai "github.com/lansonsam/eliis/internal/backend/openai"
	"github.com/lansonsam/eliis/internal/core/config"
	"github.com/lansonsam/eliis/internal/core/contract"
	"github.com/lansonsam/eliis/internal/protocol/anthropic"
	"github.com/lansonsam/eliis/internal/protocol/lens"
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

	router, err := buildRouter(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build router: %v\n", err)
		os.Exit(1)
	}

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

func buildRouter(cfg *config.Root) (*gin.Engine, error) {
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

	if cfg == nil {
		return r, nil
	}
	app, err := newApp(cfg)
	if err != nil {
		return nil, err
	}
	r.POST("/v1/messages", app.handleAnthropicMessages)

	return r, nil
}

type app struct {
	anthropicCodec *anthropic.Codec
	backend        contract.Backend
	lensChain      contract.LensChain
}

func newApp(cfg *config.Root) (*app, error) {
	if cfg.Routes.AnthropicMessages.Backend != "openai" {
		return nil, fmt.Errorf("unsupported anthropic_messages backend %q", cfg.Routes.AnthropicMessages.Backend)
	}
	timeout, err := cfg.Upstreams.OpenAI.TimeoutDuration()
	if err != nil {
		return nil, err
	}
	backend := backendopenai.New(backendopenai.Config{
		BaseURL: cfg.Upstreams.OpenAI.BaseURL,
		APIKey:  cfg.Upstreams.OpenAI.APIKey,
		Timeout: timeout,
	})
	return &app{
		anthropicCodec: anthropic.NewCodec(),
		backend:        backend,
		lensChain: contract.LensChain{
			lens.OverrideModel{Model: cfg.Routes.AnthropicMessages.Model},
			lens.EnsureMaxTokens{Default: cfg.Routes.AnthropicMessages.DefaultMaxTokens},
		},
	}, nil
}

func (a *app) handleAnthropicMessages(c *gin.Context) {
	req, err := a.anthropicCodec.DecodeRequest(c.Request)
	if err != nil {
		anthropic.WriteError(c.Writer, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if req.Stream {
		anthropic.WriteError(c.Writer, http.StatusBadRequest, "invalid_request_error", "streaming is not implemented yet")
		return
	}
	if err := a.lensChain.Apply(req); err != nil {
		anthropic.WriteError(c.Writer, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	resp, err := a.backend.Invoke(c.Request.Context(), req)
	if err != nil {
		status, typ := mapBackendError(err)
		anthropic.WriteError(c.Writer, status, typ, err.Error())
		return
	}
	if err := a.anthropicCodec.EncodeResponse(resp, c.Writer); err != nil {
		anthropic.WriteError(c.Writer, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
}

func mapBackendError(err error) (int, string) {
	var upstream *backendopenai.UpstreamError
	if !errors.As(err, &upstream) {
		return http.StatusBadGateway, "api_error"
	}
	switch upstream.StatusCode {
	case http.StatusBadRequest:
		return http.StatusBadRequest, "invalid_request_error"
	case http.StatusUnauthorized:
		return http.StatusUnauthorized, "authentication_error"
	case http.StatusForbidden:
		return http.StatusForbidden, "permission_error"
	case http.StatusNotFound:
		return http.StatusNotFound, "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return http.StatusRequestEntityTooLarge, "request_too_large"
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests, "rate_limit_error"
	default:
		return http.StatusBadGateway, "api_error"
	}
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
