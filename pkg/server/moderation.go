package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/moderation"
)

// censorExistingMessages applies the filter to message content that predates it.
//
// Censoring at post time cannot reach what is already stored, so without this a
// server that switches the filter on — or adds a term — keeps serving the old
// text forever. Running it on every startup also means a change to extra_words
// takes effect on history at the next restart, with no separate command to run.
//
// The cost is one pass over the Message table per boot. That is the same order as
// the work MemDB already does loading every message into memory immediately
// afterwards, and rows that are already clean are read but not rewritten. Set
// backfill_on_start = false to skip it once history is known to be clean.
func censorExistingMessages(db *database.DB, f *moderation.Filter) error {
	start := time.Now()
	stats, err := db.CensorMessages(context.Background(), f)
	if err != nil {
		return fmt.Errorf("word filter backfill: %w", err)
	}

	if stats.Changed > 0 {
		log.Printf("Word filter: censored %d of %d existing messages in %s",
			stats.Changed, stats.Scanned, time.Since(start).Round(time.Millisecond))
	} else {
		log.Printf("Word filter: scanned %d existing messages, nothing to censor (%s)",
			stats.Scanned, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

// newWordFilter builds the word filter described by config, returning nil when
// filtering is switched off. A term the operator got wrong is an error rather
// than a warning: a filter that silently is not filtering is worse than one that
// refuses to start.
func newWordFilter(config ServerConfig) (*moderation.Filter, error) {
	if !config.WordFilterEnabled {
		return nil, nil
	}
	filter, err := moderation.New(moderation.Options{
		ExtraWords:  config.WordFilterExtra,
		RemoveWords: config.WordFilterRemove,
		AllowWords:  config.WordFilterAllow,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid [moderation] word list: %w", err)
	}
	return filter, nil
}

// censorContent runs posted content through the word filter, returning the text
// that should actually be stored.
//
// It is called on the way in — before the message reaches the database — so that
// every path out is covered by construction: the NEW_MESSAGE broadcast,
// LIST_MESSAGES history, the archive service's own database, and the static HTML
// the archiver publishes. Filtering on the way out would mean six separate call
// sites and a new one to forget every time a message-bearing feature is added.
//
// The trade-off: the original text is not kept in the database. When logging is
// enabled it goes to server.log instead, together with the author's nickname and
// IP address, which doubles as the only place an anonymous poster's IP is
// visible and therefore as the record you need in order to ban them.
func (s *Server) censorContent(sess *Session, content string) string {
	censored, changed := s.wordFilter.Censor(content)
	if !changed || !s.config.WordFilterLog {
		return censored
	}

	sess.mu.RLock()
	nickname := sess.Nickname
	sess.mu.RUnlock()

	host, _, err := net.SplitHostPort(sess.RemoteAddr)
	if err != nil {
		host = sess.RemoteAddr
	}

	log.Printf("[filter] session %d (%s, ip=%s) posted filtered content; original: %q",
		sess.ID, nickname, host, content)
	return censored
}
