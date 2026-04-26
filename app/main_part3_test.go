package main

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestBackupDB(t *testing.T) {

	fileSize := func(t *testing.T, path string) int64 {
		t.Helper()
		info, err := os.Stat(path)
		require.NoError(t, err)
		return info.Size()
	}

	readFile := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(data)
	}

	t.Run("no backup if max 0", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		err := backupDB(dbFile, "v1", 0)
		require.NoError(t, err)
		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("skip existing backup", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		backupFile := dbFile + ".master-123-20250108T00:01:26"
		require.NoError(t, os.WriteFile(backupFile, []byte("old backup"), 0o600))
		origSize := fileSize(t, backupFile)

		err := backupDB(dbFile, "master-123-20250108T00:01:26", 1)
		require.NoError(t, err)

		newSize := fileSize(t, backupFile)
		require.Equal(t, origSize, newSize, "backup file should not be modified")
	})

	t.Run("make new backup and cleanup", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		oldBackups := []string{
			"master-111-20250108T00:01:26",
			"master-222-20250108T00:02:26",
			"master-333-20250108T00:03:26",
		}
		for _, v := range oldBackups {
			require.NoError(t, os.WriteFile(dbFile+"."+v, []byte("old"), 0o600))
		}

		newVer := "master-444-20250108T00:04:26"
		err := backupDB(dbFile, newVer, 2)
		require.NoError(t, err)

		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Len(t, files, 2)

		sort.Strings(files)
		for _, f := range files {
			base := filepath.Base(f)
			require.True(t, strings.HasSuffix(base, oldBackups[2]) || strings.HasSuffix(base, newVer),
				"unexpected file: %s", base)
		}

		content := readFile(t, dbFile+"."+newVer)
		require.Equal(t, "test data", content)
	})

	t.Run("mixed_formats", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		require.NoError(t, os.WriteFile(dbFile+".master-aaa-20250101T12:00:00", []byte("1"), 0o600))
		require.NoError(t, os.WriteFile(dbFile+".master-bbb-20250101T13:00:00", []byte("2"), 0o600))

		testTime := time.Date(2025, 1, 1, 12, 30, 0, 0, time.Local)
		require.NoError(t, os.WriteFile(dbFile+".backup1", []byte("3"), 0o600))
		require.NoError(t, os.Chtimes(dbFile+".backup1", testTime, testTime))

		err := backupDB(dbFile, "master-ccc-20250101T14:00:00", 3)
		require.NoError(t, err)

		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Len(t, files, 3)

		foundFiles := make(map[string]bool)
		for _, f := range files {
			foundFiles[filepath.Base(f)] = true
			t.Logf("found file: %s", filepath.Base(f))
		}

		require.True(t, foundFiles["test.db.master-ccc-20250101T14:00:00"], "newest versioned backup")
		require.True(t, foundFiles["test.db.master-bbb-20250101T13:00:00"], "middle versioned backup")
		require.True(t, foundFiles["test.db.backup1"], "normal backup with mod time in between")

		_, err = os.Stat(dbFile + ".master-aaa-20250101T12:00:00")
		require.True(t, os.IsNotExist(err), "oldest versioned file should be gone")
	})

	t.Run("version with dots", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		version := "master-123-1.2.3-20250108T00:01:26"
		err := backupDB(dbFile, version, 1)
		require.NoError(t, err)

		expectedBackup := dbFile + "." + strings.ReplaceAll(version, ".", "_")
		_, err = os.Stat(expectedBackup)
		require.NoError(t, err)

		content := readFile(t, expectedBackup)
		require.Equal(t, "test data", content)

		require.Contains(t, expectedBackup, "master-123-1_2_3-20250108T00:01:26")
		require.NotContains(t, expectedBackup, "master-123-1.2.3-20250108T00:01:26")
	})

	t.Run("backup with no db file", func(t *testing.T) {
		dir := t.TempDir()
		nonExistentFile := filepath.Join(dir, "non-existent.db")
		err := backupDB(nonExistentFile, "v1", 1)
		require.NoError(t, err)
	})
}
