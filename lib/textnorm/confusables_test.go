package textnorm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfusablesFold_Basic(t *testing.T) {
	n := New(Options{ConfusablesFold: true, LowerCase: true})
	assert.Equal(t, "password", n.Normalize("pаsswоrd"))
}

func TestConfusablesFold_Greek(t *testing.T) {
	n := New(Options{ConfusablesFold: true, LowerCase: true})
	result := n.Normalize("ΑΒΓ")
	assert.Equal(t, "abγ", result)
}

func TestConfusablesFold_Idempotent(t *testing.T) {
	n := New(Options{ConfusablesFold: true})
	input := "hello world"
	assert.Equal(t, input, n.Normalize(input))
}

func TestConfusablesFold_Off(t *testing.T) {
	n := New(Options{ConfusablesFold: false, LowerCase: true})
	input := "pаsswоrd"
	assert.NotEqual(t, "password", n.Normalize(input))
}

func TestNFKC_Basic(t *testing.T) {
	n := New(Options{NFKC: true})
	result := n.Normalize("ﬁ")
	assert.Equal(t, "fi", result)
}

func TestNFKC_Off(t *testing.T) {
	n := New(Options{NFKC: false})
	input := "ﬁ"
	assert.Equal(t, input, n.Normalize(input))
}

func TestConfusables_LatintoCyrillic(t *testing.T) {
	mixed := "aаbсcԁdeе"
	n := New(Options{ConfusablesFold: true, LowerCase: true})
	result := n.Normalize(mixed)
	expected := strings.Repeat("a", 2) + strings.Repeat("b", 1) + strings.Repeat("c", 2) + strings.Repeat("d", 2) + strings.Repeat("e", 2)
	assert.Equal(t, expected, result)
}

func TestPipelineOrder(t *testing.T) {
	n := New(Options{
		StripInvisible:      true,
		NFKC:                true,
		ConfusablesFold:     true,
		LowerCase:           true,
		CanonicalWhitespace: true,
	})
	result := n.Normalize("  P\u200BАSS\u0406WORD  ")
	assert.Equal(t, "passiword", result)
}
