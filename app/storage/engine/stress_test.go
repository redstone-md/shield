package engine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStress_ConcurrentWrites(t *testing.T) {
	db, err := NewSqlite(t.TempDir()+"/stress.db", "gr1")
	require.NoError(t, err)
	defer db.Close()
	_, _ = db.Exec("PRAGMA journal_mode=WAL")

	_, err = db.Exec(`CREATE TABLE stress_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		value INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	const workers = 50
	const opsPerWorker = 100
	lock := db.MakeLock()

	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range opsPerWorker {
				name := fmt.Sprintf("worker_%d_op_%d", id, j)
				lock.Lock()
				_, errExec := db.Exec("INSERT INTO stress_test (gid, name, value) VALUES (?, ?, ?)", "gr1", name, id*j)
				lock.Unlock()
				if errExec != nil {
					select {
					case errCh <- fmt.Errorf("worker %d insert %d: %w", id, j, errExec):
					default:
					}
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent write failed: %v", err)
	}

	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM stress_test")
	require.NoError(t, err)
	assert.Equal(t, workers*opsPerWorker, count)
}

func TestStress_ConcurrentReadWrite(t *testing.T) {
	db, err := NewSqlite(t.TempDir()+"/stress_rw.db", "gr1")
	require.NoError(t, err)
	defer db.Close()
	_, _ = db.Exec("PRAGMA journal_mode=WAL")

	_, err = db.Exec(`CREATE TABLE stress_rw (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL
	)`)
	require.NoError(t, err)

	for i := range 100 {
		_, err := db.Exec("INSERT INTO stress_rw (gid, data) VALUES (?, ?)", "gr1", fmt.Sprintf("seed_%d", i))
		require.NoError(t, err)
	}

	const readers = 20
	const writers = 10
	const opsPer = 50
	lock := db.MakeLock()

	var wg sync.WaitGroup
	errCh := make(chan error, readers+writers)

	for i := range readers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range opsPer {
				lock.RLock()
				var count int
				err := db.Get(&count, "SELECT COUNT(*) FROM stress_rw")
				lock.RUnlock()
				if err != nil {
					select {
					case errCh <- fmt.Errorf("reader %d: %w", id, err):
					default:
					}
					return
				}
			}
		}(i)
	}

	for i := range writers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range opsPer {
				lock.Lock()
				_, err := db.Exec("INSERT INTO stress_rw (gid, data) VALUES (?, ?)", "gr1", fmt.Sprintf("w_%d_%d", id, j))
				lock.Unlock()
				if err != nil {
					select {
					case errCh <- fmt.Errorf("writer %d: %w", id, err):
					default:
					}
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent rw failed: %v", err)
	}

	var count int
	require.NoError(t, db.Get(&count, "SELECT COUNT(*) FROM stress_rw"))
	assert.Equal(t, 100+writers*opsPer, count)
}

func TestStress_TableInitConcurrent(t *testing.T) {
	db, err := NewSqlite(t.TempDir()+"/stress.db", "gr1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	const concurrency = 20
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)
	lock := db.MakeLock()

	for i := range concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tableName := fmt.Sprintf("stress_init_%d", id)
			lock.Lock()
			_, err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				gid TEXT NOT NULL DEFAULT '',
				data TEXT
			)`, tableName))
			lock.Unlock()
			if err != nil {
				select {
				case errCh <- fmt.Errorf("init %d: %w", id, err):
				default:
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent table init failed: %v", err)
	}

	for i := range concurrency {
		var exists bool
		tableName := fmt.Sprintf("stress_init_%d", i)
		err := db.Get(&exists, "SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name=?", tableName)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist", tableName)
	}
}
