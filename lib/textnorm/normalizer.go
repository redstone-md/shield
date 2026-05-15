package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ScriptFoldFunc maps a string to a script-folded form (for example, mapping look-alike scripts to Latin).
type ScriptFoldFunc func(string) string

// Options configures which normalization steps a Normalizer applies and in which order.
type Options struct {
	LowerCase           bool
	Trim                bool
	StripInvisible      bool
	CanonicalWhitespace bool
	NFKC                bool
	ConfusablesFold     bool
	ScriptFold          ScriptFoldFunc
}

// Normalizer applies the configured text normalization pipeline to input strings.
type Normalizer struct {
	opts Options
}

// New builds a Normalizer that runs the steps enabled by opts.
func New(opts Options) Normalizer {
	return Normalizer{opts: opts}
}

// Default returns a Normalizer preconfigured for case-insensitive whitespace-canonical lookup.
func Default() Normalizer {
	return New(Options{
		LowerCase:           true,
		Trim:                true,
		StripInvisible:      true,
		CanonicalWhitespace: true,
	})
}

// Normalize applies the configured normalization steps to text and returns the result.
func (n Normalizer) Normalize(text string) string {
	if n.opts.StripInvisible {
		text = stripInvisible(text)
	}
	if n.opts.NFKC {
		text = norm.NFKC.String(text)
	}
	if n.opts.ConfusablesFold {
		text = foldConfusables(text)
	}
	if n.opts.ScriptFold != nil {
		text = n.opts.ScriptFold(text)
	}
	if n.opts.LowerCase {
		text = strings.ToLower(text)
	}
	if n.opts.CanonicalWhitespace {
		text = strings.Join(strings.Fields(text), " ")
	} else if n.opts.Trim {
		text = strings.TrimSpace(text)
	}
	return text
}

func stripInvisible(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	for _, r := range text {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if (r >= 0x200B && r <= 0x200F) || (r >= 0x2060 && r <= 0x206F) {
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
