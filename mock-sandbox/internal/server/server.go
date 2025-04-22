package server

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Server represents the HTTP server
type Server struct {
	router *mux.Router
	logger *zap.Logger
	server *http.Server
}

// NewServer creates a new HTTP server
func NewServer(logger *zap.Logger) *Server {
	router := mux.NewRouter()

	server := &Server{
		router: router,
		logger: logger,
		server: &http.Server{
			Addr:    ":8000",
			Handler: router,
		},
	}

	// Register routes
	server.registerRoutes()

	return server
}

// registerRoutes sets up all the HTTP routes
func (s *Server) registerRoutes() {
	// Health check endpoint
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")

	// Ready check endpoint
	s.router.HandleFunc("/ready", s.readyHandler).Methods("GET")

	// Crash endpoint
	s.router.HandleFunc("/crash", s.crashHandler).Methods("GET")

	// Exit endpoint
	s.router.HandleFunc("/exit", s.exitHandler).Methods("GET")

	// Slow shutdown endpoint
	s.router.HandleFunc("/slow-shutdown", s.slowShutdownHandler).Methods("GET")

	// Root endpoint
	s.router.HandleFunc("/", s.rootHandler).Methods("GET")
}

// healthHandler returns a 200 OK for health checks
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// readyHandler returns a 200 OK for readiness checks
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

// crashHandler crashes the application by panicking
func (s *Server) crashHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Received crash request")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Crashing..."))

	// Use a goroutine to ensure the response is sent before crashing
	go func() {
		time.Sleep(100 * time.Millisecond)
		panic("Intentionally crashing the application")
	}()
}

// exitHandler exits the application with code 0
func (s *Server) exitHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Received exit request")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Exiting gracefully..."))

	// Use a goroutine to ensure the response is sent before exiting
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
}

// slowShutdownHandler simulates a slow shutdown by sleeping for a while
func (s *Server) slowShutdownHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Received slow shutdown request")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Initiating slow shutdown..."))

	// Use a goroutine to ensure the response is sent before sleeping
	go func() {
		// Sleep for 30 seconds to simulate a slow shutdown
		s.logger.Info("Sleeping for 30 seconds to simulate slow shutdown")
		time.Sleep(30 * time.Second)
		s.logger.Info("Slow shutdown sleep completed, exiting")
		os.Exit(0)
	}()
}

// rootHandler returns basic information about the sandbox
func (s *Server) rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Sandbox is running"))
}

// Start starts the HTTP server
func (s *Server) Start(lc fx.Lifecycle) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			s.logger.Info("Starting HTTP server on port 8000")

			// Start the server in a goroutine
			go func() {
				if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					s.logger.Error("Server error", zap.Error(err))
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			s.logger.Info("Stopping HTTP server")

			// Create a context with timeout for graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			// Attempt graceful shutdown
			if err := s.server.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("Error during server shutdown", zap.Error(err))
				return err
			}

			s.logger.Info("HTTP server stopped")
			return nil
		},
	})
}

// Module exports the server module for fx
var Module = fx.Options(
	fx.Provide(NewServer),
	fx.Invoke(func(s *Server, lc fx.Lifecycle) {
		s.Start(lc)
	}),
)
