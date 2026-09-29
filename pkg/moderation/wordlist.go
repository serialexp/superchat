package moderation

// This file contains the built-in term list for the word filter. It is a list
// of racial and ethnic slurs, spelled out, because a filter cannot match what it
// cannot name. Nothing here is an endorsement of anything; it exists so the
// server can replace these words with asterisks before they reach anyone.
//
// Adjusting the list is a server operator decision, not a code change:
//
//	[moderation]
//	extra_words  = ["..."]   # add terms
//	remove_words = ["coon"]  # drop a built-in term
//	allow_words  = ["..."]   # never censor these whole words
//
// Scope is deliberately narrow: unambiguous racial and ethnic slurs. Other
// categories of abuse (homophobic, ableist, general profanity) are not included
// and should be added via extra_words if a server wants them, so that each
// operator makes that call explicitly.

// termSpec describes one entry of the built-in list.
type termSpec struct {
	word string

	// substring matches the term anywhere inside a word instead of requiring
	// separators on both sides. Use it only for stems where every word
	// containing them is also a slur, and cover the exceptions in
	// builtinAllowWords.
	substring bool

	// collision marks a term with a legitimate English use that context-free
	// matching cannot distinguish. These keep word-boundary semantics even where
	// matching is otherwise relaxed — see Filter.MatchesName, which matches
	// everything else anywhere in a nickname but would otherwise refuse
	// "RaccoonFan" and "Spice_Girl".
	collision bool
}

// builtinTerms is the default list.
//
// Entries marked COLLISION have a legitimate English use that this filter will
// also star out, because the match is on the word itself and no amount of
// context-free matching can separate the two. They are included because the
// slur use is far more common in a chat room than the innocent one, but an
// operator who disagrees can drop any of them with remove_words.
var builtinTerms = []termSpec{
	// Matched as a substring so compounds are caught without enumerating them:
	// sandnigger, niggerlover, nigga, niggaz, niggah, and so on. The English
	// words that legitimately contain this stem are in builtinAllowWords.
	{word: "nigg", substring: true},

	{word: "kike"}, {word: "kikes"},
	{word: "wetback"}, {word: "wetbacks"},
	{word: "beaner"}, {word: "beaners"},
	{word: "gook"}, {word: "gooks"},
	{word: "towelhead"}, {word: "towelheads"}, {word: "towel head"},
	{word: "raghead"}, {word: "ragheads"}, {word: "rag head"},
	{word: "pickaninny"}, {word: "pickaninnies"},
	{word: "chinaman"}, {word: "chinamen"},
	{word: "zipperhead"}, {word: "zipperheads"},
	{word: "spearchucker"}, {word: "spear chucker"},
	{word: "junglebunny"}, {word: "jungle bunny"},
	{word: "porchmonkey"}, {word: "porch monkey"},
	{word: "tarbaby"}, {word: "tar baby"},
	{word: "mudshark"},
	{word: "sandmonkey"}, {word: "sand monkey"},
	{word: "cameljockey"}, {word: "camel jockey"},
	{word: "currymuncher"}, {word: "curry muncher"},
	{word: "darkie"}, {word: "darkies"}, {word: "darky"},
	{word: "halfbreed"}, {word: "half breed"},
	{word: "halfcaste"}, {word: "half caste"},
	{word: "kaffir"}, {word: "kaffirs"}, {word: "kafir"},
	{word: "golliwog"}, {word: "gollywog"},
	{word: "wog"}, {word: "wogs"},
	{word: "negress"},
	{word: "mulatto"}, {word: "mulattos"}, {word: "mulattoes"},
	{word: "octoroon"}, {word: "quadroon"},
	{word: "sambo"}, {word: "sambos"},
	{word: "zambo"},
	{word: "gyppo"}, {word: "gippo"}, {word: "gypo"},
	{word: "abbo"}, {word: "abbos"},
	{word: "injun"},
	{word: "heeb"}, {word: "heebs"},
	{word: "hymie"},
	{word: "jewboy"}, {word: "jew boy"},
	{word: "christkiller"}, {word: "christ killer"},
	{word: "honky"}, {word: "honkie"}, {word: "honkies"}, {word: "honkey"},
	{word: "paki"}, {word: "pakis"},
	{word: "dago"}, {word: "dagos"}, {word: "dagoes"},
	{word: "yid"}, {word: "yids"},

	// COLLISION: "a chink in the armour"
	{word: "chink", collision: true}, {word: "chinks", collision: true},
	// COLLISION: Maine Coon, raccoon, surname Coon
	{word: "coon", collision: true}, {word: "coons", collision: true},
	// COLLISION: "spic and span", spice
	{word: "spic", collision: true}, {word: "spics", collision: true},
	{word: "spick", collision: true}, {word: "spicks", collision: true},
	// COLLISION: doo-wop
	{word: "wop", collision: true}, {word: "wops", collision: true},
	// COLLISION: dated abbreviation, Japan
	{word: "jap", collision: true}, {word: "japs", collision: true},
	// COLLISION: potatoes, peanuts
	{word: "redskin", collision: true}, {word: "redskins", collision: true},
	// COLLISION: North American place names
	{word: "squaw", collision: true}, {word: "squaws", collision: true},
}

// builtinAllowWords are whole words that are never censored even though they
// contain a built-in term. Every one of these exists because of the "nigg"
// substring entry: they are ordinary English words that happen to share the
// stem, and starring them would be a plain bug.
var builtinAllowWords = []string{
	"niggard",
	"niggardly",
	"niggardliness",
	"niggardness",
	"niggle",
	"niggles",
	"niggled",
	"niggling",
	"nigglingly",
	"niggler",
	"snigger",
	"sniggers",
	"sniggered",
	"sniggering",
}
