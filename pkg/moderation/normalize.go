package moderation

import (
	"unicode"
	"unicode/utf8"
)

// Normalized bytes produced by normalizeRune. Every rune of the input maps to
// exactly one of these, except dropped runes which produce no byte at all.
const (
	// sepSpace marks whitespace. It is a hard boundary: matching never reaches
	// across it, which is what stops a term from being assembled out of the
	// initials of an innocent sentence ("walk I kept" must not yield "kike").
	sepSpace byte = 0x00

	// sepPunct marks any other non-letter: punctuation, emoji, digits with no
	// letter lookalike. It is a soft boundary, skippable between the letters of
	// a term, which is what catches "n.i.g.g.e.r" and "n-i-g-g-e-r".
	sepPunct byte = 0x02

	// opaque marks a letter that cannot be folded onto ASCII (Han, Hebrew,
	// Devanagari, ...). It counts as a word character so it does not create a
	// false word boundary, but no term can ever contain it.
	opaque byte = 0x01

	// drop means the rune carries no meaning for matching and should not
	// occupy a slot at all. Used for zero-width characters and combining
	// marks, both of which are common evasion tricks ("nig<ZWSP>ger", "nïgger").
	drop byte = 0xFF

	// wildcard appears only inside a term, never in normalized text. It stands
	// for "one or more separators", which is how a space in a multi-word term
	// such as "porch monkey" is matched.
	wildcard byte = ' '
)

// isSep reports whether b is either class of separator, i.e. whether it ends a
// word for boundary-matching purposes.
func isSep(b byte) bool { return b == sepSpace || b == sepPunct }

// asciiConfusables maps ASCII non-letters that are routinely substituted for a
// letter onto that letter. Digits without a convincing lookalike ('2', '6') are
// deliberately absent and fall through to sepPunct.
//
// These runes have a dual nature, and getting it wrong costs real matches in
// both directions. Read purely as letters, they stop being word boundaries, and
// "you kike!" no longer matches a word-mode term — a false negative in the most
// likely way for a slur to ever be typed. Read purely as punctuation, "n!gg3r"
// stops matching. So normalizeRune reports them as the letter *and* flags them
// as soft, and matching treats a soft slot as a letter for comparison and as a
// boundary for anchoring. See scratch.soft.
var asciiConfusables = map[rune]byte{
	'0': 'o',
	'1': 'i',
	'3': 'e',
	'4': 'a',
	'5': 's',
	'7': 't',
	'8': 'b',
	'9': 'g',
	'@': 'a',
	'$': 's',
	'!': 'i',
	'|': 'i',
	'+': 't',
}

// confusables maps non-ASCII runes onto the ASCII letter they are drawn like.
// Keys are lowercase: the lookup happens after unicode.ToLower, so only one
// entry per pair is needed.
//
// Latin-1 accented forms cover the "níggér" dodge. The Cyrillic and Greek
// entries cover homoglyph substitution, where a visually identical character
// from another script is swapped in ("nigger" with a Cyrillic е).
var confusables = map[rune]byte{
	// Latin-1 and Latin Extended-A
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ė': 'e', 'ę': 'e', 'ě': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ĩ': 'i', 'ī': 'i', 'ĭ': 'i', 'į': 'i', 'ı': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ŏ': 'o', 'ő': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ũ': 'u', 'ū': 'u', 'ŭ': 'u', 'ů': 'u', 'ű': 'u', 'ų': 'u',
	'ý': 'y', 'ÿ': 'y', 'ŷ': 'y',
	'ñ': 'n', 'ń': 'n', 'ņ': 'n', 'ň': 'n',
	'ç': 'c', 'ć': 'c', 'ĉ': 'c', 'ċ': 'c', 'č': 'c',
	'ĝ': 'g', 'ğ': 'g', 'ġ': 'g', 'ģ': 'g',
	'ś': 's', 'ŝ': 's', 'ş': 's', 'š': 's', 'ș': 's',
	'ţ': 't', 'ť': 't', 'ț': 't',
	'ź': 'z', 'ż': 'z', 'ž': 'z',
	'ŕ': 'r', 'ř': 'r',
	'ĺ': 'l', 'ľ': 'l', 'ł': 'l',
	'ď': 'd', 'đ': 'd',
	'ķ': 'k',
	'ĥ': 'h',
	'ĵ': 'j',
	'ŵ': 'w',
	'þ': 'b',

	// Cyrillic homoglyphs
	'а': 'a', 'в': 'b', 'с': 'c', 'ԁ': 'd', 'е': 'e', 'н': 'h', 'і': 'i', 'ј': 'j',
	'к': 'k', 'м': 'm', 'о': 'o', 'р': 'p', 'ѕ': 's', 'т': 't', 'у': 'y', 'х': 'x',

	// Greek homoglyphs
	'α': 'a', 'β': 'b', 'ε': 'e', 'η': 'n', 'ι': 'i', 'κ': 'k', 'ν': 'v',
	'ο': 'o', 'ρ': 'p', 'σ': 's', 'τ': 't', 'υ': 'u', 'χ': 'x',
}

// zeroWidth are invisible runes used to break up a word without changing how it
// looks. They are dropped so that "nig<ZWJ>ger" normalizes the same as "nigger".
// Written as code points rather than literals: these runes are invisible, and a
// source file nobody can proofread is a source file nobody can maintain.
var zeroWidth = map[rune]bool{
	0x00ad: true, // soft hyphen
	0x200b: true, // zero width space
	0x200c: true, // zero width non-joiner
	0x200d: true, // zero width joiner
	0x2060: true, // word joiner
	0xfeff: true, // zero width no-break space / BOM
}

// normalizeRune folds a rune to the single ASCII letter it represents, or to
// sepSpace, sepPunct, opaque, or drop. See the constants above for what each
// means.
//
// soft reports that the rune is a non-letter standing in for a letter (a digit
// or a symbol): it compares as the letter, but still counts as a word boundary.
//
// Not covered, deliberately: the Mathematical Alphanumeric Symbols blocks
// (𝓷𝓲𝓰𝓰𝓮𝓻 and friends). They are a long tail of sparse, hole-ridden ranges,
// and someone reaching for them is no longer "obviously" anything.
func normalizeRune(r rune) (b byte, soft bool) {
	if r < utf8.RuneSelf {
		switch {
		case r >= 'a' && r <= 'z':
			return byte(r), false
		case r >= 'A' && r <= 'Z':
			return byte(r-'A') + 'a', false
		}
		if b, ok := asciiConfusables[r]; ok {
			return b, true
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			return sepSpace, false
		}
		return sepPunct, false
	}

	if zeroWidth[r] {
		return drop, false
	}
	// Combining marks decorate the preceding letter rather than standing on
	// their own, so they must not split a word.
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return drop, false
	}

	lower := unicode.ToLower(r)
	if lower >= 'a' && lower <= 'z' {
		return byte(lower), false
	}
	if b, ok := confusables[lower]; ok {
		// A homoglyph is a letter in its own script, not a stand-in symbol.
		return b, false
	}
	// Fullwidth Latin letters (ｎｉｇｇｅｒ).
	if lower >= 0xff41 && lower <= 0xff5a {
		return byte(lower-0xff41) + 'a', false
	}
	if unicode.IsLetter(lower) {
		return opaque, false
	}
	if unicode.IsDigit(lower) {
		return opaque, true
	}
	if unicode.IsSpace(lower) {
		return sepSpace, false
	}
	return sepPunct, false
}

// scratch holds the normalized form of one message plus the mapping back to the
// original string, so a match found in normalized space can be rewritten in the
// original. It is pooled and reused: Censor runs on every posted message and
// must not allocate per message when nothing matches.
type scratch struct {
	norm []byte  // one byte per surviving rune
	off  []int32 // norm[i] came from s[off[i] : off[i]+size[i]]
	size []int32

	// soft[i] marks a slot whose source rune was a digit or symbol standing in
	// for a letter ('1' for i, '!' for i, '@' for a). Such a slot compares as
	// its letter but also terminates a word, so that "n!gg3r" matches and
	// "you kike!" still has a boundary after the term.
	soft []bool
}

func (sc *scratch) reset(s string) {
	sc.norm = sc.norm[:0]
	sc.off = sc.off[:0]
	sc.size = sc.size[:0]
	sc.soft = sc.soft[:0]

	for i := 0; i < len(s); {
		// DecodeRuneInString reports the true width of invalid bytes as 1,
		// which utf8.RuneLen(RuneError) would get wrong.
		r, n := utf8.DecodeRuneInString(s[i:])
		if b, soft := normalizeRune(r); b != drop {
			sc.norm = append(sc.norm, b)
			sc.off = append(sc.off, int32(i))
			sc.size = append(sc.size, int32(n))
			sc.soft = append(sc.soft, soft)
		}
		i += n
	}
}

// boundaryAt reports whether index i ends a word: either it is past the end, or
// it holds a separator, or it holds a symbol standing in for a letter.
func (sc *scratch) boundaryAt(i int) bool {
	if i < 0 || i >= len(sc.norm) {
		return true
	}
	return isSep(sc.norm[i]) || sc.soft[i]
}
