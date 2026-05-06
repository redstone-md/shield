package observability

import (
	"bytes"
	"context"
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefix_CompactMetadata(t *testing.T) {
	ctx := WithModerationMetadata(context.Background(), "evt-1", "corr-1", "telegram:update:1")

	assert.Equal(t, "evt=evt-1 corr=corr-1 idem=telegram:update:1 ", Prefix(ctx))
}

func TestLogf_PreservesLevelWithMetadata(t *testing.T) {
	var buf bytes.Buffer
	restore := captureStdLog(t, &buf)
	defer restore()

	ctx := WithModerationMetadata(context.Background(), "evt-1", "corr-1", "telegram:update:1")
	Logf(ctx, "[DEBUG] incoming msg: %s", "hello")

	assert.Equal(t, "[DEBUG] evt=evt-1 corr=corr-1 idem=telegram:update:1 incoming msg: hello\n", buf.String())
}

func TestLogf_DefaultsToMetadataPrefixWithoutLevel(t *testing.T) {
	var buf bytes.Buffer
	restore := captureStdLog(t, &buf)
	defer restore()

	ctx := WithEventMetadata(context.Background(), "evt-2", "corr-2")
	Logf(ctx, "message without explicit level")

	assert.Equal(t, "evt=evt-2 corr=corr-2 message without explicit level\n", buf.String())
}

func captureStdLog(t *testing.T, buf *bytes.Buffer) func() {
	t.Helper()

	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()

	log.SetOutput(buf)
	log.SetFlags(0)
	log.SetPrefix("")

	return func() {
		restoreLogOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}
}

func restoreLogOutput(writer io.Writer) {
	log.SetOutput(writer)
}
