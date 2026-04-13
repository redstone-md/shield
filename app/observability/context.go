package observability

import (
	"context"
	"fmt"
	"log"
)

type metadataKey struct{}

// Metadata carries correlation fields for the moderation flow.
type Metadata struct {
	EventID       string
	CorrelationID string
}

// WithEventMetadata stores correlation metadata in a context.
func WithEventMetadata(ctx context.Context, eventID, correlationID string) context.Context {
	return context.WithValue(ctx, metadataKey{}, Metadata{
		EventID:       eventID,
		CorrelationID: correlationID,
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
	if meta.EventID == "" && meta.CorrelationID == "" {
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
	return fmt.Sprintf("event_id=%s correlation_id=%s ", meta.EventID, meta.CorrelationID)
}

// Logf writes a log line prefixed with correlation metadata when present.
func Logf(ctx context.Context, format string, args ...any) {
	if prefix := Prefix(ctx); prefix != "" {
		log.Printf(prefix+format, args...)
		return
	}
	log.Printf(format, args...)
}
