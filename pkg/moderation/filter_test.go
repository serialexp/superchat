package moderation

import (
	"strings"
	"testing"
	"testing/quick"
)

// Invisible characters used in evasion tests, spelled out by code point so the
// test source stays readable.
var (
	zwsp               = string(rune(0x200b)) // zero width space
	combiningDiaeresis = string(rune(0x0308)) // combining mark over the previous letter
	cyrillicO          = string(rune(0x043e)) // Cyrillic о, drawn like Latin o
)

func mustNew(t *testing.T, opts Options) *Filter {
	t.Helper()
	f, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func TestCensorCleanTextIsUntouched(t *testing.T) {
	f := mustNew(t, Options{})

	clean := []string{
		"",
		"hello world",
		"Anyone around? I had a question about the protocol docs.",
		"the niggardly landlord raised the rent again",
		"she sniggered at the joke",
		"that detail has been niggling at me all week",
		"I found a chigger bite on my ankle",
		"Nigeria and Niger are different countries",
		"raccoons got into the bins, and the cocoon is intact",
		"we spent the weekend in Japan",
		"package main\n\nfunc main() {}\n",
		"1234567890 !@#$%^&*()",
		"日本語のテキスト",
	}

	for _, in := range clean {
		got, changed := f.Censor(in)
		if changed {
			t.Errorf("Censor(%q) = %q, reported a change; want untouched", in, got)
		}
		if got != in {
			t.Errorf("Censor(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestCensorReplacesTerms(t *testing.T) {
	f := mustNew(t, Options{})

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare term", "nigger", "******"},
		{"in a sentence", "you are a nigger", "you are a ******"},
		{"uppercase", "NIGGER", "******"},
		{"mixed case", "NiGgEr", "******"},
		{"plural via substring stem", "niggers", "*******"},
		{"variant spelling", "nigga", "*****"},
		{"variant spelling z", "niggaz", "******"},
		{"compound prefix", "sandnigger", "**********"},
		{"compound suffix", "niggerlover", "***********"},
		{"separate term", "kike", "****"},
		{"word mode plural", "kikes", "*****"},
		{"multi word term", "porch monkey", "***** ******"},
		{"multi word hyphenated", "porch-monkey", "*****-******"},
		{"leetspeak", "n1gg3r", "******"},
		{"leet with symbols", "n!gg3r", "******"},
		{"accented", "níggér", "******"},
		{"zero width evasion", "nig" + zwsp + "ger", "******"},
		{"combining marks", "ni" + combiningDiaeresis + "gger", "******"},
		{"fullwidth", "ｎｉｇｇｅｒ", "******"},
		{"keeps surrounding text", "hey nigger, listen", "hey ******, listen"},
		{"two matches", "kike and nigger", "**** and ******"},
		{"trailing punctuation", "nigger!", "******!"},
		{"multiline", "line one\nnigger\nline three", "line one\n******\nline three"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := f.Censor(tt.in)
			if !changed {
				t.Fatalf("Censor(%q) reported no change", tt.in)
			}
			if got != tt.want {
				t.Errorf("Censor(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCensorCyrillicHomoglyph(t *testing.T) {
	f := mustNew(t, Options{})

	// Cyrillic о (U+043E) substituted for the Latin o in "gook".
	in := "g" + cyrillicO + "ok"
	got, changed := f.Censor(in)
	if !changed {
		t.Fatalf("Censor(%q) reported no change; homoglyph evasion not caught", in)
	}
	if got != "****" {
		t.Errorf("Censor(%q) = %q, want %q", in, got, "****")
	}
}

func TestWordModeRequiresBoundaries(t *testing.T) {
	f := mustNew(t, Options{})

	// "spic" is word-mode, so it must not fire inside a longer word.
	for _, in := range []string{"spice", "spices", "spicy", "auspicious", "conspicuous"} {
		got, changed := f.Censor(in)
		if changed {
			t.Errorf("Censor(%q) = %q; word-mode term fired inside a longer word", in, got)
		}
	}
}

func TestAllowWordsBeatSubstringTerms(t *testing.T) {
	f := mustNew(t, Options{})

	// Built-in allowlist entries containing the "nigg" stem.
	for _, in := range []string{"niggardly", "niggling", "snigger", "sniggering"} {
		if got, changed := f.Censor(in); changed {
			t.Errorf("Censor(%q) = %q; allowlisted word was censored", in, got)
		}
	}

	// A word sharing the stem but not allowlisted is still censored whole.
	got, changed := f.Censor("niggerish")
	if !changed || got != strings.Repeat("*", len("niggerish")) {
		t.Errorf("Censor(%q) = %q, changed=%v; want fully starred", "niggerish", got, changed)
	}
}

func TestExtraWords(t *testing.T) {
	f := mustNew(t, Options{ExtraWords: []string{"frobnicate", "two words"}})

	got, changed := f.Censor("please frobnicate the widget")
	if !changed || got != "please ********** the widget" {
		t.Errorf("Censor = %q, changed=%v", got, changed)
	}

	got, _ = f.Censor("two words here")
	if got != "*** ***** here" {
		t.Errorf("multi-word extra term: got %q", got)
	}

	// Extra words are word-mode, not substring.
	if _, changed := f.Censor("frobnicated"); changed {
		t.Error("extra word matched as a substring; want word-mode")
	}
}

func TestRemoveWords(t *testing.T) {
	f := mustNew(t, Options{RemoveWords: []string{"chink", "chinks"}})

	if _, changed := f.Censor("a chink in the armour"); changed {
		t.Error("removed built-in term still matched")
	}
	// Removing one term leaves the rest alone.
	if _, changed := f.Censor("kike"); !changed {
		t.Error("unrelated built-in term stopped matching after remove_words")
	}
}

func TestAllowWordsOption(t *testing.T) {
	f := mustNew(t, Options{AllowWords: []string{"coon"}})

	if _, changed := f.Censor("a Maine Coon cat"); changed {
		t.Error("configured allow word was censored")
	}
}

func TestDisableBuiltins(t *testing.T) {
	f := mustNew(t, Options{DisableBuiltins: true, ExtraWords: []string{"badger"}})

	if _, changed := f.Censor("nigger"); changed {
		t.Error("built-in term matched with DisableBuiltins set")
	}
	if _, changed := f.Censor("badger"); !changed {
		t.Error("extra word did not match with DisableBuiltins set")
	}
}

// Soft separators (punctuation) are skipped between the letters of a term;
// whitespace is not. That asymmetry is the whole defence against assembling a
// term out of the initials of an innocent sentence.
func TestSeparatorClasses(t *testing.T) {
	f := mustNew(t, Options{})

	punctuated := []struct{ in, want string }{
		{"n.i.g.g.e.r", "*.*.*.*.*.*"},
		{"n-i-g-g-e-r", "*-*-*-*-*-*"},
		{"n_i_g_g_e_r", "*_*_*_*_*_*"},
		{"k.i.k.e", "*.*.*.*"},
	}
	for _, tt := range punctuated {
		got, changed := f.Censor(tt.in)
		if !changed || got != tt.want {
			t.Errorf("Censor(%q) = %q (changed=%v), want %q", tt.in, got, changed, tt.want)
		}
	}

	// Whitespace must not be crossed, or ordinary sentences start matching.
	for _, in := range []string{
		"n i g g e r",
		"walk I kept going",       // spells k-i-k-e across words
		"the top is picked clean", // spells s-p-i-c across words
	} {
		if got, changed := f.Censor(in); changed {
			t.Errorf("Censor(%q) = %q; matched across whitespace", in, got)
		}
	}
}

// Punctuation must keep working as a word boundary. When '!' was folded to the
// letter 'i', "you kike!" had no boundary after the term and silently failed to
// match — the most likely way for a slur to actually be typed.
func TestTerminalPunctuationStillBounds(t *testing.T) {
	f := mustNew(t, Options{})

	tests := []struct{ in, want string }{
		{"you kike!", "you ****!"},
		{"you kike?", "you ****?"},
		{"gook.", "****."},
		{"\"gook\"", "\"****\""},
		{"(gook)", "(****)"},
		{"gook, really", "****, really"},
		{"kike!!!", "****!!!"},
	}
	for _, tt := range tests {
		got, changed := f.Censor(tt.in)
		if !changed || got != tt.want {
			t.Errorf("Censor(%q) = %q (changed=%v), want %q", tt.in, got, changed, tt.want)
		}
	}
}

// Leftward expansion must not swallow the preceding word.
func TestCensorDoesNotSwallowNeighbouringWords(t *testing.T) {
	f := mustNew(t, Options{})

	got, _ := f.Censor("hello,nigger")
	if got != "hello,******" {
		t.Errorf("Censor(%q) = %q, want %q", "hello,nigger", got, "hello,******")
	}
}

func TestNilFilterIsNoOp(t *testing.T) {
	var f *Filter
	got, changed := f.Censor("nigger")
	if changed || got != "nigger" {
		t.Errorf("nil filter: got %q, changed=%v; want passthrough", got, changed)
	}
	if f.NumTerms() != 0 {
		t.Errorf("nil filter NumTerms = %d, want 0", f.NumTerms())
	}
}

func TestNewRejectsUnusableTerms(t *testing.T) {
	// Terms that normalize to nothing matchable, or to letters this filter
	// cannot represent. ("!!!" is not in this list: '!' reads as 'i', so it is a
	// usable if peculiar term.)
	for _, w := range []string{".,;", "###", "日本", "-"} {
		if _, err := New(Options{DisableBuiltins: true, ExtraWords: []string{w}}); err == nil {
			t.Errorf("New accepted unusable term %q", w)
		}
	}

	// Whitespace-only entries are skipped rather than rejected, so that a
	// stray blank line in a config file is not a fatal error.
	f, err := New(Options{DisableBuiltins: true, ExtraWords: []string{"   ", "kike"}})
	if err != nil {
		t.Fatalf("New rejected a blank entry: %v", err)
	}
	if f.NumTerms() != 1 {
		t.Errorf("NumTerms = %d, want 1", f.NumTerms())
	}
}

func TestBuiltinListIsWellFormed(t *testing.T) {
	f := mustNew(t, Options{})
	if f.NumTerms() < 50 {
		t.Errorf("NumTerms = %d, want the built-in list to be loaded", f.NumTerms())
	}

	// Every built-in term must actually censor itself. This catches a typo in
	// the list that would otherwise sit there matching nothing.
	for _, spec := range builtinTerms {
		got, changed := f.Censor(spec.word)
		if !changed {
			t.Errorf("built-in term %q does not match itself", spec.word)
			continue
		}
		if strings.ContainsAny(got, "abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("built-in term %q censored to %q, letters left over", spec.word, got)
		}
	}

	// Every allowlist entry must survive the filter.
	for _, w := range builtinAllowWords {
		if got, changed := f.Censor(w); changed {
			t.Errorf("allowlist entry %q was censored to %q", w, got)
		}
	}
}

// TestCensorPreservesNonMatchingBytes is the safety property that matters most:
// whatever the filter does, it must not corrupt text it did not match.
func TestCensorPreservesNonMatchingBytes(t *testing.T) {
	f := mustNew(t, Options{})

	check := func(s string) bool {
		got, changed := f.Censor(s)
		if !changed {
			return got == s
		}
		// Censoring only ever swaps word runes for '*' one-for-one or drops
		// invisible runes, so the rune count can only shrink by the invisibles.
		return len([]rune(got)) <= len([]rune(s))
	}

	if err := quick.Check(check, &quick.Config{MaxCount: 2000}); err != nil {
		t.Error(err)
	}
}

func TestCensorIsIdempotent(t *testing.T) {
	f := mustNew(t, Options{})

	for _, in := range []string{"nigger", "hey kike", "porch monkey", "n1gg3r and gook"} {
		once, _ := f.Censor(in)
		twice, changed := f.Censor(once)
		if changed {
			t.Errorf("Censor(%q) = %q, which still matched on a second pass -> %q", in, once, twice)
		}
	}
}

func BenchmarkCensorClean(b *testing.B) {
	f, err := New(Options{})
	if err != nil {
		b.Fatal(err)
	}
	// Representative chat message: nothing matches, which is the hot path.
	msg := "Has anyone else seen the reconnect loop when the server restarts? " +
		"I get about three retries and then it gives up entirely."

	b.SetBytes(int64(len(msg)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, changed := f.Censor(msg); changed {
			b.Fatal("benchmark message should not match")
		}
	}
}

func BenchmarkCensorMaxLengthClean(b *testing.B) {
	f, err := New(Options{})
	if err != nil {
		b.Fatal(err)
	}
	// A message at the 4096-byte protocol limit, the worst realistic case.
	msg := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 4096/44)

	b.SetBytes(int64(len(msg)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Censor(msg)
	}
}

func BenchmarkCensorMatch(b *testing.B) {
	f, err := New(Options{})
	if err != nil {
		b.Fatal(err)
	}
	msg := "hey nigger you kike, also gook"

	b.SetBytes(int64(len(msg)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Censor(msg)
	}
}
