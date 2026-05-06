package observability

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type metadataKey struct{}

// Metadata carries correlation fields for the moderation flow.
type Metadata struct {
	EventID        string
	CorrelationID  string
	IdempotencyKey string
}

// WithEventMetadata stores correlation metadata in a context.
func WithEventMetadata(ctx context.Context, eventID, correlationID string) context.Context {
	return context.WithValue(ctx, metadataKey{}, Metadata{
		EventID:       eventID,
		CorrelationID: correlationID,
	})
}

// WithModerationMetadata stores correlation and idempotency metadata in a context.
func WithModerationMetadata(ctx context.Context, eventID, correlationID, idempotencyKey string) context.Context {
	return context.WithValue(ctx, metadataKey{}, Metadata{
		EventID:        eventID,
		CorrelationID:  correlationID,
		IdempotencyKey: idempotencyKey,
	})
}

// MetadataFromContext returns correlation metadata from a context.
func MetadataFromContext(ctx context.Context) (Metadata, bool) {
	if ctx == nil {
		return Metadata{}, false
	}
	meta, ok := ctx.Value(metadataKey{}).(Metadata)
	if !ok {
		return Metadata{}, false
	}
	if meta.EventID == "" && meta.CorrelationID == "" && meta.IdempotencyKey == "" {
		return Metadata{}, false
	}
	return meta, true
}

// Prefix returns a stable log prefix when metadata is present.
func Prefix(ctx context.Context) string {
	meta, ok := MetadataFromContext(ctx)
	if !ok {
		return ""
	}
	prefix := fmt.Sprintf("evt=%s corr=%s ", meta.EventID, meta.CorrelationID)
	if meta.IdempotencyKey != "" {
		prefix += fmt.Sprintf("idem=%s ", meta.IdempotencyKey)
	}
	return prefix
}

// Logf writes a log line prefixed with correlation metadata when present.
func Logf(ctx context.Context, format string, args ...any) {
	if prefix := Prefix(ctx); prefix != "" {
		log.Printf(withMetadataPrefix(format, prefix), args...)
		return
	}
	log.Printf(format, args...)
}

func withMetadataPrefix(format, prefix string) string {
	for _, level := range []string{"[TRACE]", "[DEBUG]", "[INFO]", "[WARN]", "[ERROR]", "[PANIC]", "[FATAL]"} {
		if strings.HasPrefix(format, level) {
			return level + " " + prefix + strings.TrimSpace(format[len(level):])
		}
	}
	return prefix + format
}
