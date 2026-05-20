package storage

import (
	"context"
	"github.com/go-pkgz/testutils/containers"
	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/stretchr/testify/suite"
	"os"
	"path/filepath"
	"testing"
)

type StorageTestSuite struct {
	suite.Suite
	dbs         map[string]*engine.SQL
	pgContainer *containers.PostgresTestContainer
	sqliteFile  string
}

func TestStorageSuite(t *testing.T) {
	suite.Run(t, new(StorageTestSuite))
}

func (s *StorageTestSuite) SetupSuite() {
	s.dbs = make(map[string]*engine.SQL)

	s.sqliteFile = filepath.Join(os.TempDir(), "test.db")
	s.T().Logf("sqlite file: %s", s.sqliteFile)
	sqliteDB, err := engine.NewSqlite(s.sqliteFile, "gr1")
	s.Require().NoError(err)
	s.dbs["sqlite"] = sqliteDB

	if !testing.Short() {
		s.T().Log("start postgres container")
		ctx := context.Background()

		s.pgContainer = containers.NewPostgresTestContainerWithDB(ctx, s.T(), "test")
		s.T().Log("postgres container started")

		connStr := s.pgContainer.ConnectionString()
		pgDB, err := engine.NewPostgres(ctx, connStr, "gr1")
		s.Require().NoError(err)
		s.dbs["postgres"] = pgDB
	}
}

func (s *StorageTestSuite) TearDownSuite() {
	for _, db := range s.dbs {
		db.Close()
	}
	if s.pgContainer != nil {
		s.T().Log("terminating container")
		s.pgContainer.Close(context.Background())
	}
	if s.sqliteFile != "" {
		s.T().Logf("removing sqlite file: %s", s.sqliteFile)
		err := os.Remove(s.sqliteFile)
		s.Require().NoError(err)
	}
}

func (s *StorageTestSuite) getTestDB() []struct {
	DB   *engine.SQL
	Type engine.Type
} {
	res := make([]struct {
		DB   *engine.SQL
		Type engine.Type
	}, 0, len(s.dbs))
	for name, db := range s.dbs {
		res = append(res, struct {
			DB   *engine.SQL
			Type engine.Type
		}{
			DB:   db,
			Type: engine.Type(name),
		})
	}
	return res
}

func TestPrepareStoreURL(t *testing.T) {
	testCases := []struct {
		name     string
		connURL  string
		wantType engine.Type
		wantConn string
		wantErr  bool
	}{
		{
			name:     "sqlite file",
			connURL:  "test.db",
			wantType: engine.Sqlite,
			wantConn: "test.db",
			wantErr:  false,
		},
		{
			name:     "sqlite file with extension",
			connURL:  "test.sqlite",
			wantType: engine.Sqlite,
			wantConn: "test.sqlite",
			wantErr:  false,
		},
		{
			name:     "sqlite url",
			connURL:  "sqlite:/path/to/db",
			wantType: engine.Sqlite,
			wantConn: "file:/path/to/db",
			wantErr:  false,
		},
		{
			name:     "sqlite3 url",
			connURL:  "sqlite3:/path/to/db",
			wantType: engine.Sqlite,
			wantConn: "file:/path/to/db",
			wantErr:  false,
		},
		{
			name:     "memory",
			connURL:  "memory",
			wantType: engine.Sqlite,
			wantConn: ":memory:",
			wantErr:  false,
		},
		{
			name:     "memory url",
			connURL:  "memory://",
			wantType: engine.Sqlite,
			wantConn: ":memory:",
			wantErr:  false,
		},
		{
			name:     "mem url",
			connURL:  "mem://",
			wantType: engine.Sqlite,
			wantConn: ":memory:",
			wantErr:  false,
		},
		{
			name:     "in-memory",
			connURL:  "file::memory:",
			wantType: engine.Sqlite,
			wantConn: ":memory:",
			wantErr:  false,
		},
		{
			name:     "postgres url",
			connURL:  "postgres://user:pass@host:5432/db",
			wantType: engine.Postgres,
			wantConn: "postgres://user:pass@host:5432/db",
			wantErr:  false,
		},
		{
			name:     "mysql without parseTime",
			connURL:  "user:pass@tcp(host:3306)/db",
			wantType: engine.Mysql,
			wantConn: "user:pass@tcp(host:3306)/db?parseTime=true",
			wantErr:  false,
		},
		{
			name:     "mysql with other params",
			connURL:  "user:pass@tcp(host:3306)/db?charset=utf8",
			wantType: engine.Mysql,
			wantConn: "user:pass@tcp(host:3306)/db?charset=utf8&parseTime=true",
			wantErr:  false,
		},
		{
			name:     "mysql with parseTime",
			connURL:  "user:pass@tcp(host:3306)/db?parseTime=true",
			wantType: engine.Mysql,
			wantConn: "user:pass@tcp(host:3306)/db?parseTime=true",
			wantErr:  false,
		},
		{
			name:     "unsupported url",
			connURL:  "invalid-url",
			wantType: engine.Unknown,
			wantConn: "",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbType, conn, err := prepareStoreURL(tc.connURL)
			if tc.wantErr {
				if err == nil {
					t.Errorf("prepareStoreURL(%q) returned no error, expected an error", tc.connURL)
				}
				return
			}
			if err != nil {
				t.Errorf("prepareStoreURL(%q) returned unexpected error: %v", tc.connURL, err)
			}
			if dbType != tc.wantType {
				t.Errorf("prepareStoreURL(%q) returned dbType = %v, want %v", tc.connURL, dbType, tc.wantType)
			}
			if conn != tc.wantConn {
				t.Errorf("prepareStoreURL(%q) returned conn = %q, want %q", tc.connURL, conn, tc.wantConn)
			}
		})
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()

	t.Run("sqlite in-memory", func(t *testing.T) {
		db, err := New(ctx, "memory", "test-group")
		if err != nil {
			t.Fatalf("New(ctx, 'memory', 'test-group') failed: %v", err)
		}
		defer db.Close()

		if db.Type() != engine.Sqlite {
			t.Errorf("New(ctx, 'memory', 'test-group') returned type = %v, want %v", db.Type(), engine.Sqlite)
		}
	})

	t.Run("sqlite file", func(t *testing.T) {
		tmpFile := filepath.Join(os.TempDir(), "test-new-func.db")
		defer os.Remove(tmpFile)

		db, err := New(ctx, tmpFile, "test-group")
		if err != nil {
			t.Fatalf("New(ctx, %q, 'test-group') failed: %v", tmpFile, err)
		}
		defer db.Close()

		if db.Type() != engine.Sqlite {
			t.Errorf("New(ctx, %q, 'test-group') returned type = %v, want %v", tmpFile, db.Type(), engine.Sqlite)
		}
	})

	t.Run("invalid url", func(t *testing.T) {
		_, err := New(ctx, "invalid-url", "test-group")
		if err == nil {
			t.Error("New(ctx, 'invalid-url', 'test-group') returned no error, expected an error")
		}
	})

	if !testing.Short() {

		t.Skip("skipping postgres test in standalone mode - already tested in suite")
	}
}
