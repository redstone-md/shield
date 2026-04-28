package controlplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaService_CheckAlwaysAllows(t *testing.T) {
	svc := NewQuotaService()
	ok, err := svc.Check(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestQuotaService_IncrementLogsUsage(t *testing.T) {
	svc := NewQuotaService()
	err := svc.Increment(context.Background(), "tenant-1", "throughput")
	require.NoError(t, err)
}

func TestQuotaService_MultipleLimitTypes(t *testing.T) {
	svc := NewQuotaService()
	types := []string{"throughput", "report", "llm", "retention"}
	for _, lt := range types {
		ok, err := svc.Check(context.Background(), "tenant-1", lt)
		require.NoError(t, err)
		assert.True(t, ok, "limit type %s should be allowed", lt)

		err = svc.Increment(context.Background(), "tenant-1", lt)
		require.NoError(t, err)
	}
}
