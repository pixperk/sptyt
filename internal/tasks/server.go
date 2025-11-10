package tasks

import (
	"crypto/tls"
	"log"

	"github.com/hibiken/asynq"
	"github.com/pixperk/sptyt/internal/services"
)

// Server wraps Asynq server for processing tasks
type Server struct {
	server    *asynq.Server
	mux       *asynq.ServeMux
	processor *PlaylistConversionProcessor
}

// NewServer creates a new task server
func NewServer(redisAddr, redisPassword string, converterService *services.PlaylistConverterService, concurrency int) *Server {
	redisOpt := asynq.RedisClientOpt{
		Addr: redisAddr,
	}
	if redisPassword != "" {
		redisOpt.Username = "default" // Upstash requires username
		redisOpt.Password = redisPassword
		// Enable TLS for Upstash (or any production Redis with TLS)
		redisOpt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: concurrency, // Number of concurrent workers
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)

	mux := asynq.NewServeMux()
	processor := NewPlaylistConversionProcessor(converterService)

	// Register task handlers
	mux.HandleFunc(TypePlaylistConversion, processor.ProcessPlaylistConversion)
	mux.HandleFunc(TypeAnalyticsUpdate, processor.ProcessAnalyticsUpdate)

	return &Server{
		server:    server,
		mux:       mux,
		processor: processor,
	}
}

// Start starts the task server
func (s *Server) Start() error {
	log.Println("Asynq task server starting...")
	return s.server.Run(s.mux)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() {
	log.Println("Shutting down Asynq task server...")
	s.server.Shutdown()
}
