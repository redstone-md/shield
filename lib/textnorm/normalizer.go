package textnorm

import (
	"strings"
	"unicode"
)

// ScriptFoldFunc optionally rewrites text across scripts before downstream matching.
type ScriptFoldFunc func(string) string

// Options control normalization stages.
type Options struct {
	LowerCase           bool
	Trim                bool
	StripInvisible      bool
	CanonicalWhitespace bool
	ScriptFold          ScriptFoldFunc
}

// Normalizer applies a stable text-normalization pipeline.
type Normalizer struct {
	opts Options
}

// New creates a normalizer with the provided options.
func New(opts Options) Normalizer {
	return Normalizer{opts: opts}
}

// Default returns the runtime ingress normalization pipeline.
func Default() Normalizer {
	return New(Options{
		LowerCase:           true,
		Trim:                true,
		StripInvisible:      true,
		CanonicalWhitespace: true,
	})
}

// Normalize applies the configured stages in order.
func (n Normalizer) Normalize(text string) string {
	if n.opts.StripInvisible {
		text = stripInvisible(text)
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
