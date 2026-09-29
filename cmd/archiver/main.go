package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aeolun/superchat/pkg/archiver"
)

var Version = "dev"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	listen := flag.String("listen", ":6470", "Address to listen on for archive connections")
	dbPath := flag.String("db", "archive.db", "Path to archive SQLite database")
	outputDir := flag.String("output", "./archive-html", "Output directory for static HTML")
	htmlInterval := flag.Int("html-interval", 300, "HTML regeneration interval in seconds (0 = only after backfill)")
	wordFilter := flag.Bool("word-filter", true, "Censor racial slurs in archived content")
	wordFilterBackfill := flag.Bool("word-filter-backfill", true, "On startup, censor messages already in the archive")
	wordFilterExtra := flag.String("word-filter-extra", "", "Comma-separated extra terms to censor")
	wordFilterRemove := flag.String("word-filter-remove", "", "Comma-separated built-in terms to stop censoring")
	wordFilterAllow := flag.String("word-filter-allow", "", "Comma-separated words to never censor")
	version := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *version {
		fmt.Printf("SuperChat Archiver %s\n", Version)
		os.Exit(0)
	}

	cfg := archiver.Config{
		ListenAddr:          *listen,
		DBPath:              *dbPath,
		OutputDir:           *outputDir,
		HTMLIntervalSeconds: *htmlInterval,
		WordFilter:          *wordFilter,
		WordFilterBackfill:  *wordFilterBackfill,
		WordFilterExtra:     splitList(*wordFilterExtra),
		WordFilterRemove:    splitList(*wordFilterRemove),
		WordFilterAllow:     splitList(*wordFilterAllow),
	}

	srv, err := archiver.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create archiver: %v", err)
	}

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down archiver...")
		srv.Stop()
	}()

	log.Printf("SuperChat Archiver %s starting on %s", Version, *listen)
	if err := srv.Start(); err != nil {
		log.Fatalf("Archiver error: %v", err)
	}
}

// splitList parses a comma-separated flag value into a trimmed, non-empty list.
// Only commas separate, so multi-word terms like "porch monkey" still work.
func splitList(val string) []string {
	if strings.TrimSpace(val) == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
