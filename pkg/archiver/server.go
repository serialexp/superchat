package archiver

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/aeolun/superchat/pkg/moderation"
)

// Config holds archiver configuration.
type Config struct {
	ListenAddr          string
	DBPath              string
	OutputDir           string
	HTMLIntervalSeconds int

	// Moderation. The archiver filters independently of the superchat server
	// feeding it: it holds its own copy of every message and publishes it as
	// public HTML, so it cannot rely on the server having been configured to
	// censor. WordFilterBackfill cleans messages archived before the filter.
	WordFilter         bool
	WordFilterBackfill bool
	WordFilterExtra    []string
	WordFilterRemove   []string
	WordFilterAllow    []string
}

// Server is the archive service that receives messages from superchat servers
// and stores them permanently in SQLite + generates static HTML.
type Server struct {
	config   Config
	listener net.Listener
	store    *Store
	htmlGen  *HTMLGenerator
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// New creates a new archiver server.
func New(cfg Config) (*Server, error) {
	var filter *moderation.Filter
	if cfg.WordFilter {
		f, err := moderation.New(moderation.Options{
			ExtraWords:  cfg.WordFilterExtra,
			RemoveWords: cfg.WordFilterRemove,
			AllowWords:  cfg.WordFilterAllow,
		})
		if err != nil {
			return nil, fmt.Errorf("archiver: invalid word list: %w", err)
		}
		filter = f
		log.Printf("[archiver] word filter enabled (%d terms)", filter.NumTerms())
	}

	store, err := NewStore(cfg.DBPath, filter)
	if err != nil {
		return nil, err
	}

	// Clean content archived before the filter existed. This has to happen
	// before Start generates HTML, or the published pages keep the old text.
	if filter != nil && cfg.WordFilterBackfill {
		start := time.Now()
		stats, err := store.CensorExisting(context.Background())
		if err != nil {
			store.Close()
			return nil, fmt.Errorf("archiver: word filter backfill: %w", err)
		}
		if stats.Changed > 0 {
			log.Printf("[archiver] word filter: censored %d of %d archived messages in %s",
				stats.Changed, stats.Scanned, time.Since(start).Round(time.Millisecond))
		} else {
			log.Printf("[archiver] word filter: scanned %d archived messages, nothing to censor (%s)",
				stats.Scanned, time.Since(start).Round(time.Millisecond))
		}
	}

	htmlGen := NewHTMLGenerator(cfg.OutputDir, store)

	return &Server{
		config:   cfg,
		store:    store,
		htmlGen:  htmlGen,
		shutdown: make(chan struct{}),
	}, nil
}

// Start begins listening for archive connections. Blocks until Stop is called.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return err
	}
	s.listener = ln

	// Generate initial HTML so nginx has something to serve immediately
	s.htmlGen.GenerateAll()

	// Start periodic HTML generation if configured
	if s.config.HTMLIntervalSeconds > 0 {
		s.wg.Add(1)
		go s.htmlGen.RunPeriodic(s.config.HTMLIntervalSeconds, s.shutdown, &s.wg)
	}

	log.Printf("[archiver] listening on %s", s.config.ListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return nil
			default:
				log.Printf("[archiver] accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			h := NewHandler(conn, s.store, s.htmlGen)
			h.Serve(s.shutdown)
		}()
	}
}

// Stop gracefully shuts down the archiver.
func (s *Server) Stop() {
	close(s.shutdown)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	s.store.Close()
}
