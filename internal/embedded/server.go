package embedded

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// Server wraps an embedded NATS server with JetStream enabled
type Server struct {
	server *server.Server
	opts   *server.Options
	mu     sync.RWMutex
}

// Config holds the configuration for the embedded server
type Config struct {
	Port       int
	StoreDir   string
	ServerName string
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		Port:       4222,
		StoreDir:   "./",
		ServerName: "nats-desktop-embedded",
	}
}

// New creates a new embedded server with the given configuration
func New(config Config) (*Server, error) {
	opts := &server.Options{
		ServerName: config.ServerName,
		Host:       "127.0.0.1",
		Port:       config.Port,
		JetStream:  true,
		StoreDir:   config.StoreDir,
		NoLog:      true,
		NoSigs:     true,
	}

	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded server: %w", err)
	}

	return &Server{
		server: s,
		opts:   opts,
	}, nil
}

// Start starts the embedded server and waits for it to be ready
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server.ReadyForConnections(0) {
		return fmt.Errorf("server is already running")
	}

	go s.server.Start()

	if !s.server.ReadyForConnections(5 * time.Second) {
		s.server.Shutdown()
		return fmt.Errorf("server failed to start within timeout")
	}

	return nil
}

// Stop gracefully shuts down the embedded server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		s.server.Shutdown()
		s.server.WaitForShutdown()
	}
}

// IsRunning returns true if the server is running and ready for connections
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return false
	}

	return s.server.ReadyForConnections(0)
}

// ClientURL returns the URL clients should use to connect to this server
func (s *Server) ClientURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return ""
	}

	return s.server.ClientURL()
}

// Port returns the port the server is listening on
func (s *Server) Port() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.opts == nil {
		return 0
	}

	return s.opts.Port
}

// StoreDir returns the JetStream store directory
func (s *Server) StoreDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.opts == nil {
		return ""
	}

	return s.opts.StoreDir
}
