// Package moderation provides server-side content moderation for SuperChat.
//
// The only thing here today is a word filter, which replaces matched terms with
// asterisks. It runs on the way in — message content is censored before it is
// stored — so that every consumer downstream is covered by construction: the
// live broadcast, LIST_MESSAGES, the archive service's own database, and the
// static HTML the archiver publishes.
package moderation

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

// Options configures a Filter. The zero value yields the built-in list with
// default matching.
type Options struct {
	// ExtraWords are additional terms to censor. A space in a term matches any
	// run of separators, so "porch monkey" also catches "porch-monkey".
	ExtraWords []string

	// RemoveWords drops terms from the built-in list. Compared after
	// normalization, so the spelling only has to match in skeleton form.
	RemoveWords []string

	// AllowWords are whole words that are never censored, even when they
	// contain a term. This is what keeps "niggardly" and "snigger" intact.
	AllowWords []string

	// DisableBuiltins leaves only ExtraWords, for a server that wants to supply
	// its own list wholesale.
	DisableBuiltins bool
}

// matchMode decides whether a term needs word boundaries around it.
type matchMode uint8

const (
	matchWord      matchMode = iota // separators required on both sides
	matchSubstring                  // matches anywhere inside a word
)

type term struct {
	norm []byte // normalized, with wildcard standing in for spaces
	mode matchMode

	// collision marks a term with a legitimate English use, which keeps its
	// word-boundary requirement even in MatchesName.
	collision bool
}

// Filter censors terms in a string. It is safe for concurrent use and a nil
// *Filter is a working no-op, so callers can hold nil when filtering is off.
type Filter struct {
	// buckets indexes terms by their first normalized byte, so scanning a
	// message only ever compares against the handful of terms that could start
	// at the current position rather than the whole list.
	buckets [256][]term

	allow    map[string]struct{}
	numTerms int

	pool sync.Pool
}

// New builds a Filter from opts.
func New(opts Options) (*Filter, error) {
	f := &Filter{allow: make(map[string]struct{})}
	f.pool.New = func() any { return new(scratch) }

	removed := make(map[string]struct{}, len(opts.RemoveWords))
	for _, w := range opts.RemoveWords {
		key, err := normalizeTerm(w)
		if err != nil {
			return nil, fmt.Errorf("remove_words: %w", err)
		}
		removed[string(key)] = struct{}{}
	}

	specs := make([]termSpec, 0, len(builtinTerms)+len(opts.ExtraWords))
	if !opts.DisableBuiltins {
		specs = append(specs, builtinTerms...)
	}
	for _, w := range opts.ExtraWords {
		if strings.TrimSpace(w) == "" {
			continue
		}
		specs = append(specs, termSpec{word: w})
	}

	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		norm, err := normalizeTerm(spec.word)
		if err != nil {
			return nil, fmt.Errorf("word %q: %w", spec.word, err)
		}
		key := string(norm)
		if _, skip := removed[key]; skip {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		mode := matchWord
		if spec.substring {
			mode = matchSubstring
		}
		f.buckets[norm[0]] = append(f.buckets[norm[0]], term{
			norm:      norm,
			mode:      mode,
			collision: spec.collision,
		})
		f.numTerms++
	}

	allowWords := opts.AllowWords
	if !opts.DisableBuiltins {
		allowWords = append(append([]string{}, builtinAllowWords...), allowWords...)
	}
	for _, w := range allowWords {
		if strings.TrimSpace(w) == "" {
			continue
		}
		norm, err := normalizeTerm(w)
		if err != nil {
			return nil, fmt.Errorf("allow_words %q: %w", w, err)
		}
		// Allowlist keys are compared against a literal slice of normalized
		// text, so the wildcard has to become the separator byte it stands for.
		// A multi-word entry therefore matches a single separator between words.
		for i, b := range norm {
			if b == wildcard {
				norm[i] = sepSpace
			}
		}
		f.allow[string(norm)] = struct{}{}
	}

	return f, nil
}

// normalizeTerm folds a configured term into the same skeleton form that
// message text is normalized to, keeping interior spaces as wildcards.
func normalizeTerm(w string) ([]byte, error) {
	trimmed := strings.TrimSpace(w)
	out := make([]byte, 0, len(trimmed))
	for _, r := range trimmed {
		if r == ' ' {
			// Collapse runs of spaces: one wildcard already matches a run of
			// separators.
			if len(out) > 0 && out[len(out)-1] != wildcard {
				out = append(out, wildcard)
			}
			continue
		}
		switch b, _ := normalizeRune(r); {
		case b == drop:
			continue
		case b == opaque:
			return nil, fmt.Errorf("contains a letter that cannot be folded to ASCII")
		case isSep(b):
			// Punctuation written inside a term would never match literally,
			// since punctuation in a message normalizes to a separator byte.
			// Treat it as a wildcard so "half-caste" behaves like "half caste".
			if len(out) > 0 && out[len(out)-1] != wildcard {
				out = append(out, wildcard)
			}
		default:
			out = append(out, b)
		}
	}
	for len(out) > 0 && out[len(out)-1] == wildcard {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("normalizes to nothing")
	}
	if out[0] == wildcard {
		return nil, fmt.Errorf("cannot start with a separator")
	}
	return out, nil
}

// Matches reports whether the filter would censor anything in s, using the same
// rules as Censor.
func (f *Filter) Matches(s string) bool {
	_, changed := f.Censor(s)
	return changed
}

// MatchesName reports whether s is unusable as a nickname or other identifier.
//
// It is stricter than Matches: a term matches anywhere in the string rather than
// only on word boundaries, so "xxkikexx" and "thegook" are caught. Padding a
// slur is the obvious next move once the bare name is refused, and unlike prose
// an identifier has no surrounding sentence for a word boundary to key off.
//
// The exception is terms marked as collisions — the handful with a real English
// use. Those keep their boundary requirement, so "RaccoonFan" and "Spice_Girl"
// stay usable while "coon" and "spic" on their own do not.
//
// This is a predicate only. Identifiers are refused, never censored: a starred
// nickname still says what it was, and every starred name of the same length
// collides with every other.
func (f *Filter) MatchesName(s string) bool {
	if f == nil || s == "" {
		return false
	}

	sc := f.pool.Get().(*scratch)
	defer f.pool.Put(sc)
	sc.reset(s)

	for i := 0; i < len(sc.norm); i++ {
		b := sc.norm[i]
		if isSep(b) {
			continue
		}
		for _, t := range f.buckets[b] {
			if matchAt(sc, i, t, true) < 0 {
				continue
			}
			// An allowlisted word still wins, so "niggardly" is a legal name.
			ws, we := wordBounds(sc, i, i+1)
			if _, ok := f.allow[string(sc.norm[ws:we])]; ok {
				continue
			}
			return true
		}
	}
	return false
}

// NumTerms reports how many terms the filter is matching against.
func (f *Filter) NumTerms() int {
	if f == nil {
		return 0
	}
	return f.numTerms
}

// Censor replaces the letters of every matched term with '*' and reports
// whether anything changed. Non-letter runes inside a match are passed through,
// so "n.i.g.g.e.r" becomes "*.*.*.*.*.*".
//
// When nothing matches — the overwhelmingly common case — the input string is
// returned as-is and no memory is allocated beyond the pooled scratch buffers.
func (f *Filter) Censor(s string) (string, bool) {
	if f == nil || s == "" {
		return s, false
	}

	sc := f.pool.Get().(*scratch)
	defer f.pool.Put(sc)
	sc.reset(s)

	var out []byte
	copied := 0 // how much of s has been written to out

	for i := 0; i < len(sc.norm); {
		b := sc.norm[i]
		if isSep(b) {
			i++
			continue
		}

		// Longest match wins, so "niggers" is censored as one word rather than
		// leaving the tail exposed.
		end := -1
		for _, t := range f.buckets[b] {
			if e := matchAt(sc, i, t, false); e > end {
				end = e
			}
		}
		if end < 0 {
			i++
			continue
		}

		// Censor the whole word the match sits in, not just the matched span.
		// For a word-mode term the two are the same; for a substring term it is
		// the difference between "**********" and "sand****er".
		ws, we := wordBounds(sc, i, end)

		// An allowlisted word wins over any term inside it.
		if _, ok := f.allow[string(sc.norm[ws:we])]; ok {
			i = we
			continue
		}

		os := int(sc.off[ws])
		oe := int(sc.off[we-1] + sc.size[we-1])
		if out == nil {
			out = make([]byte, 0, len(s))
		}
		out = append(out, s[copied:os]...)
		out = appendStars(out, s[os:oe])
		copied = oe
		i = we
	}

	if out == nil {
		return s, false
	}
	out = append(out, s[copied:]...)
	return string(out), true
}

// matchAt tries to match t against norm starting at start, returning the
// exclusive end index in norm, or -1 for no match.
//
// Soft separators (punctuation) between the letters of a term are skipped, so
// "n.i.g.g.e.r" matches. Whitespace is never skipped, which is what keeps a term
// from being assembled across word boundaries. A wildcard in the term — from a
// space in a multi-word entry like "porch monkey" — matches a run of either.
// nameMode relaxes the word-boundary requirement for every term that is not
// marked as a collision, which is what makes MatchesName catch a padded slur.
func matchAt(sc *scratch, start int, t term, nameMode bool) int {
	norm := sc.norm
	j := start
	for k := 0; k < len(t.norm); k++ {
		c := t.norm[k]

		if c == wildcard {
			if j >= len(norm) || !isSep(norm[j]) {
				return -1
			}
			for j < len(norm) && isSep(norm[j]) {
				j++
			}
			continue
		}

		if k > 0 {
			for j < len(norm) && norm[j] == sepPunct {
				j++
			}
		}
		if j >= len(norm) || norm[j] != c {
			return -1
		}
		j++
	}

	needsBoundaries := t.mode == matchWord
	if nameMode && !t.collision {
		needsBoundaries = false
	}
	if needsBoundaries && !(sc.boundaryAt(start-1) && sc.boundaryAt(j)) {
		return -1
	}
	return j
}

// wordBounds expands [start, end) outward to the word the match belongs to. That
// span is both the allowlist key and the run of text that gets starred, so that
// a substring hit censors "sandnigger" whole rather than leaving "sand****er".
//
// The two directions are deliberately not symmetric:
//
//   - Leftward it crosses letters only. Crossing punctuation too would make
//     "hello,nigger" star "hello" as well.
//   - Rightward it also crosses runs of soft slots — punctuation and
//     letter-substitute symbols — provided a real letter follows, so the tail of
//     a broken-up word is included: "n.i.g.g.e.r" does not leak ".e.r" and
//     "n1gg3r" does not leak "3r". With nothing but separators after it the run
//     is left alone, so "nigger!" stars as "******!" rather than "*******".
func wordBounds(sc *scratch, start, end int) (int, int) {
	norm := sc.norm
	for start > 0 && !sc.boundaryAt(start-1) {
		start--
	}
	for end < len(norm) {
		if !sc.boundaryAt(end) {
			end++
			continue
		}
		// Look past a run of soft slots for another letter to absorb.
		next := end
		for next < len(norm) && (norm[next] == sepPunct || sc.soft[next]) {
			next++
		}
		if next == end || next >= len(norm) || sc.boundaryAt(next) {
			break
		}
		end = next
	}
	return start, end
}

// appendStars writes one '*' per word rune in src and passes everything else
// through, dropping the invisible runes that were only there to evade matching.
func appendStars(dst []byte, src string) []byte {
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRuneInString(src[i:])
		switch b, _ := normalizeRune(r); {
		case b == drop:
			// Skip: reproducing a zero-width character serves no purpose.
		case isSep(b):
			dst = append(dst, src[i:i+n]...)
		default:
			dst = append(dst, '*')
		}
		i += n
	}
	return dst
}
