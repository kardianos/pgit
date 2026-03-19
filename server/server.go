// Package server implements the pgit HTTP server.
package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/server/api"
	"github.com/imgajeed76/pgit/v4/server/auth"
)

// Server is the pgit HTTP server.
type Server struct {
	db     *db.DB
	addr   string
	mux    *http.ServeMux
	server *http.Server
}

// New creates a new Server.
func New(d *db.DB, addr string) *Server {
	s := &Server{
		db:   d,
		addr: addr,
		mux:  http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// registerRoutes wires all API endpoints into the mux.
func (s *Server) registerRoutes() {
	h := api.NewHandler(s.db)
	authMiddleware := auth.TokenAuth(s.db)

	// Wrap the handler with auth middleware.
	authed := authMiddleware(h)

	s.mux.Handle("/", authed)
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s.server.ListenAndServe()
}

// StartOnListener begins serving on the provided listener.
// Useful for tests that need to pick an ephemeral port.
func (s *Server) StartOnListener(ln net.Listener) error {
	s.server = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s.server.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.addr
}
