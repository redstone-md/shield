package storage

import (
	"context"
	"fmt"
	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/redstone-md/shield/lib/approved"
)

func (s *StorageTestSuite) TestApprovedUsers_Cleanup() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			au, err := NewApprovedUsers(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE approved_users")

			s.Run("cleanup on migration", func() {

				_, err := db.Exec(db.Adopt("INSERT INTO approved_users (uid, name) VALUES (?, ?)"), "", "invalid")
				s.Require().NoError(err)

				au2, err := NewApprovedUsers(ctx, db)
				s.Require().NoError(err)

				users, err := au2.Read(ctx)
				s.Require().NoError(err)
				s.Empty(users)
			})

			s.Run("handle invalid data in read", func() {
				query := "INSERT INTO approved_users (uid, name, timestamp) VALUES (?, ?, ?)"
				if db.Type() == engine.Postgres {
					query = "INSERT INTO approved_users (uid, name, timestamp) VALUES ($1, $2, $3::timestamp)"
				}

				_, err := db.Exec(query, "123", "test", "invalid-time")
				if db.Type() == engine.Postgres {
					s.T().Skip("postgres prevents invalid timestamp format")
				}
				s.Require().NoError(err)

				users, err := au.Read(ctx)
				s.Require().NoError(err)
				s.Empty(users)
			})
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_Migrate() {
	ctx := context.Background()

	s.Run("migrate from old sqlite schema", func() {
		db, err := engine.NewSqlite(":memory:", "gr1")
		s.Require().NoError(err)
		defer db.Close()

		_, err = db.Exec(`
            CREATE TABLE approved_users (
                id TEXT PRIMARY KEY,
                name TEXT,
                timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
            )
        `)
		s.Require().NoError(err)

		testData := []struct {
			id   string
			name string
		}{
			{"123", "test1"},
			{"456", "test2"},
		}

		for _, tc := range testData {
			_, execErr := db.Exec("INSERT INTO approved_users (id, name) VALUES (?, ?)", tc.id, tc.name)
			s.Require().NoError(execErr)
		}

		// verify initial data
		var count int
		err = db.Get(&count, "SELECT COUNT(*) FROM approved_users")
		s.Require().NoError(err)
		s.Equal(2, count)

		au, err := NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		users, err := au.Read(ctx)
		s.Require().NoError(err)
		s.Len(users, 2)

		s.Equal("test1", users[0].UserName)
		s.Equal("123", users[0].UserID)
		s.Equal("test2", users[1].UserName)
		s.Equal("456", users[1].UserID)

		// verify table structure
		var cols []string
		rows, err := db.Query("SELECT * FROM approved_users LIMIT 1")
		s.Require().NoError(err)
		cols, err = rows.Columns()
		s.Require().NoError(err)
		rows.Close()

		expected := []string{"id", "uid", "gid", "name", "timestamp"}
		for _, col := range expected {
			s.Contains(cols, col)
		}
	})

	s.Run("no migration needed for new schema", func() {
		db, err := engine.NewSqlite(":memory:", "gr1")
		s.Require().NoError(err)
		defer db.Close()

		au1, err := NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		err = au1.Write(ctx, approved.UserInfo{UserID: "123", UserName: "test"})
		s.Require().NoError(err)

		au2, err := NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		users, err := au2.Read(ctx)
		s.Require().NoError(err)
		s.Len(users, 1)
		s.Equal("test", users[0].UserName)
		s.Equal("123", users[0].UserID)
	})

	s.Run("double migration attempt", func() {
		db, err := engine.NewSqlite(":memory:", "gr1")
		s.Require().NoError(err)
		defer db.Close()

		_, err = db.Exec(`
            CREATE TABLE approved_users (
                id TEXT PRIMARY KEY,
                name TEXT,
                timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
            )
        `)
		s.Require().NoError(err)

		_, err = db.Exec("INSERT INTO approved_users (id, name) VALUES ('123', 'test')")
		s.Require().NoError(err)

		_, err = NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		au2, err := NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		users, err := au2.Read(ctx)
		s.Require().NoError(err)
		s.Len(users, 1)
		s.Equal("test", users[0].UserName)
		s.Equal("123", users[0].UserID)
	})

	s.Run("migration preserves indices", func() {
		db, err := engine.NewSqlite(":memory:", "gr1")
		s.Require().NoError(err)
		defer db.Close()

		_, err = NewApprovedUsers(ctx, db)
		s.Require().NoError(err)

		var indices []struct {
			Name string `db:"name"`
		}
		err = db.Select(&indices, "SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'approved_users'")
		s.Require().NoError(err)

		expectedIndices := []string{
			"idx_approved_users_uid",
			"idx_approved_users_gid",
			"idx_approved_users_name",
			"idx_approved_users_timestamp",
		}

		for _, expected := range expectedIndices {
			found := false
			for _, idx := range indices {
				if idx.Name == expected {
					found = true
					break
				}
			}
			s.True(found, "index %s not found", expected)
		}
	})
}
