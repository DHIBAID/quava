package profiling

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"
)

type Config struct {
	Address              string
	MutexProfileFraction int
	BlockProfileRate     int
}

type Server struct {
	server *http.Server
	logger *slog.Logger
}

func Start(cfg Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:6060"
	}

	// Restrict profiling to localhost.
	host, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid profiling address: %w", err)
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf(
			"profiling address must be loopback: %s",
			cfg.Address,
		)
	}

	mux := http.NewServeMux()

	// Explicitly register pprof handlers.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Register individual profiles.
	mux.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	mux.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	mux.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
	mux.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	mux.HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)

	// Enable advanced runtime profiling.
	runtime.SetMutexProfileFraction(cfg.MutexProfileFraction)
	runtime.SetBlockProfileRate(cfg.BlockProfileRate)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("starting profiling listener: %w", err)
	}

	s := &Server{
		server: server,
		logger: logger,
	}

	go func() {
		logger.Info("pprof server started", "address", cfg.Address)

		if err := server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof server failed", "error", err)
		}
	}()

	return s, nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}

	s.logger.Info("stopping pprof server")

	return s.server.Shutdown(ctx)
}
