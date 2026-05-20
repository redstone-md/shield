package main

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/controlplane"
)

func TestAssembleRuntimeBootstrapsWorkspace(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	var opts options
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db"))
	opts.Files.SamplesDataPath = tmpDir
	opts.Files.DynamicDataPath = tmpDir

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	require.NotNil(t, assembly.WorkspacesStore)
	require.NotNil(t, assembly.WorkspaceService)

	ws, err := assembly.WorkspacesStore.Get(ctx, "gr1")
	require.NoError(t, err)
	assert.Equal(t, "gr1", ws.Name)
	assert.Equal(t, "tg-spam", ws.OwnerID)
	assert.Equal(t, "active", ws.Status)

	member, err := assembly.WorkspacesStore.GetMember(ctx, ws.ID, "tg-spam")
	require.NoError(t, err)
	assert.Equal(t, string(controlplane.RoleOwner), member.Role)
}
