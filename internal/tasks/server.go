package tasks

import (
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/pixperk/sptyt/internal/config"
	"github.com/pixperk/sptyt/internal/services"
)

// Server wraps Asynq server for processing tasks
type Server struct {
	server    *asynq.Server
	mux       *asynq.ServeMux
	processor *PlaylistConversionProcessor
}

// NewServer creates a new task server from Redis config
func NewServer(cfg *config.RedisConfig, converterService *services.PlaylistConverterService, concurrency int) *Server {
	redisOpt := cfg.NewAsynqRedisOpt()

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: concurrency, // Number of concurrent workers
			Queues: map[string]int{
				"default": 1, // Single queue to minimize polling
			},
			// Aggressively reduce Redis polling to minimize idle commands
			HealthCheckInterval:      5 * time.Minute,  // Default: 15s - check worker health every 5 min
			DelayedTaskCheckInterval: 5 * time.Minute,  // Default: 5s - check scheduled tasks every 5 min
			StrictPriority:           true,
		},
	)

	mux := asynq.NewServeMux()
	processor := NewPlaylistConversionProcessor(converterService)

	// Register task handlers
	mux.HandleFunc(TypePlaylistConversion, processor.ProcessPlaylistConversion)
	mux.HandleFunc(TypeAnalyticsUpdate, processor.ProcessAnalyticsUpdate)
	mux.HandleFunc(TypeRetryFailedTracks, processor.ProcessRetryFailedTracks)

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
