package storage

import (
	"context"
	"fmt"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/approved"
	"sync"
	"time"
)

func (s *StorageTestSuite) TestApprovedUsers_NewApprovedUsers() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			s.Run("create new table", func() {
				_, err := NewApprovedUsers(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE approved_users")

				// check if the table exists - use db-agnostic way
				var exists int
				err = db.Get(&exists, `SELECT COUNT(*) FROM approved_users`)
				s.Require().NoError(err)
				s.Equal(0, exists)
			})

			s.Run("table already exists", func() {
				if db.Type() != engine.Sqlite {
					s.T().Skip("skipping for non-sqlite database")
				}
				defer db.Exec("DROP TABLE approved_users")

				_, err := db.Exec(`CREATE TABLE approved_users (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    uid TEXT NOT NULL UNIQUE,
                    gid TEXT DEFAULT '',
                    name TEXT,
                    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
                )`)
				s.Require().NoError(err)

				_, err = NewApprovedUsers(ctx, db)
				s.Require().NoError(err)

				// verify that the existing structure has not changed
				var columnCount int
				err = db.Get(&columnCount, "SELECT COUNT(*) FROM pragma_table_info('approved_users')")
				s.Require().NoError(err)
				s.Equal(6, columnCount)
			})

			s.Run("nil db connection", func() {
				_, err := NewApprovedUsers(ctx, nil)
				s.Require().Error(err)
				defer db.Exec("DROP TABLE approved_users")

				s.Contains(err.Error(), "db connection is nil")
			})

			s.Run("context canceled", func() {
				canceledCtx, cancel := context.WithCancel(context.Background())
				cancel()

				_, err := NewApprovedUsers(canceledCtx, db)
				s.Require().Error(err)
				defer db.Exec("DROP TABLE approved_users")
			})

			s.Run("commit after migration preserves data", func() {
				if db.Type() != engine.Sqlite {
					s.T().Skip("skipping for non-sqlite database")
				}

				_, err := db.Exec(`
                    CREATE TABLE approved_users (
                        id TEXT PRIMARY KEY,
                        name TEXT,
                        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
                    )
                `)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE approved_users")

				// verify old schema exists
				var exists int
				err = db.Get(&exists, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='approved_users'")
				s.Require().NoError(err)
				s.Equal(1, exists)

				oldTime := time.Now().Add(-time.Hour).UTC()
				_, err = db.Exec("INSERT INTO approved_users (id, name, timestamp) VALUES (?, ?, ?)", "user1", "test", oldTime)
				s.Require().NoError(err)

				au, err := NewApprovedUsers(ctx, db)
				s.Require().NoError(err)

				users, err := au.Read(ctx)
				s.Require().NoError(err)
				s.Require().Len(users, 1)
				s.Equal("user1", users[0].UserID)
				s.Equal("test", users[0].UserName)
				s.Equal(oldTime.Unix(), users[0].Timestamp.Unix())
			})
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_Write() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			tests := []struct {
				name    string
				user    approved.UserInfo
				verify  func(s *StorageTestSuite, db *engine.SQL, user approved.UserInfo)
				wantErr bool
			}{
				{
					name: "write new user with group",
					user: approved.UserInfo{
						UserID:   "123",
						UserName: "John Doe",
					},
					verify: func(s *StorageTestSuite, db *engine.SQL, user approved.UserInfo) {
						var saved approvedUsersInfo
						query := db.Adopt("SELECT uid, name FROM approved_users WHERE uid = ?")
						err := db.Get(&saved, query, user.UserID)
						s.Require().NoError(err)
						s.Equal(user.UserName, saved.UserName)
					},
				},
				{
					name: "write user without group",
					user: approved.UserInfo{
						UserID:   "456",
						UserName: "Jane Doe",
					},
					verify: func(s *StorageTestSuite, db *engine.SQL, user approved.UserInfo) {
						var saved approvedUsersInfo
						query := db.Adopt("SELECT uid, name FROM approved_users WHERE uid = ?")
						err := db.Get(&saved, query, user.UserID)
						s.Require().NoError(err)
						s.Equal(user.UserName, saved.UserName)
					},
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					au, err := NewApprovedUsers(ctx, db)
					s.Require().NoError(err)
					defer db.Exec("DROP TABLE approved_users")

					err = au.Write(ctx, tt.user)
					if tt.wantErr {
						s.Require().Error(err)
						return
					}
					s.Require().NoError(err)

					if tt.verify != nil {
						tt.verify(s, db, tt.user)
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_Read() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE approved_users")

			au, err := NewApprovedUsers(ctx, db)
			s.Require().NoError(err)

			testTime := time.Date(2023, 10, 2, 0, 0, 0, 0, time.UTC)

			users := []approved.UserInfo{
				{UserID: "123", UserName: "John", Timestamp: testTime},
				{UserID: "456", UserName: "Jane", Timestamp: testTime},
			}
			for _, u := range users {
				writeErr := au.Write(ctx, u)
				s.Require().NoError(writeErr)
			}

			users, err = au.Read(ctx)
			s.Require().NoError(err)
			s.Require().Len(users, 2)
			s.Equal("123", users[0].UserID)
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_Delete() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE approved_users")

			au, err := NewApprovedUsers(ctx, db)
			s.Require().NoError(err)

			s.Run("delete user with group", func() {
				_, err := db.Exec("DELETE FROM approved_users")
				s.Require().NoError(err)

				user := approved.UserInfo{
					UserID:   "123",
					UserName: "John",
				}
				err = au.Write(ctx, user)
				s.Require().NoError(err)

				err = au.Delete(ctx, user.UserID)
				s.Require().NoError(err)

				var count int
				query := db.Adopt("SELECT COUNT(*) FROM approved_users WHERE uid = ?")
				err = db.Get(&count, query, user.UserID)
				s.Require().NoError(err)
				s.Equal(0, count)
			})
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_StoreAndRead() {
	tests := []struct {
		name     string
		ids      []string
		expected []string
	}{
		{
			name:     "empty",
			ids:      []string{},
			expected: []string{},
		},
		{
			name:     "single ID",
			ids:      []string{"12345"},
			expected: []string{"12345"},
		},
		{
			name:     "multiple IDs",
			ids:      []string{"123", "456", "789"},
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "multiple IDs, with one bad",
			ids:      []string{"123", "456"},
			expected: []string{"123", "456"},
		},
	}

	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			for _, tt := range tests {
				s.Run(tt.name, func() {
					defer db.Exec("DROP TABLE approved_users")

					au, err := NewApprovedUsers(ctx, db)
					s.Require().NoError(err)

					for _, id := range tt.ids {
						err = au.Write(ctx, approved.UserInfo{UserID: id, UserName: "name_" + id})
						s.Require().NoError(err)
					}

					res, err := au.Read(ctx)
					s.Require().NoError(err)
					s.Len(res, len(tt.expected))
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_ContextCancellation() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE approved_users")

			au, err := NewApprovedUsers(ctx, db)
			s.Require().NoError(err)

			s.Run("new with canceled context", func() {
				canceledCtx, cancel := context.WithCancel(context.Background())
				cancel()
				_, err := NewApprovedUsers(canceledCtx, db)
				s.Require().Error(err)
				s.Contains(err.Error(), "context canceled")
			})

			s.Run("read with canceled context", func() {

				err := au.Write(ctx, approved.UserInfo{UserID: "123", UserName: "test"})
				s.Require().NoError(err)

				ctxCanceled, cancel := context.WithCancel(context.Background())
				cancel()

				_, err = au.Read(ctxCanceled)
				s.Require().Error(err)
				s.Contains(err.Error(), "context canceled")
			})

			s.Run("write with canceled context", func() {
				ctxCanceled, cancel := context.WithCancel(context.Background())
				cancel()

				err := au.Write(ctxCanceled, approved.UserInfo{UserID: "456", UserName: "test"})
				s.Require().Error(err)
				s.Contains(err.Error(), "context canceled")
			})

			s.Run("delete with canceled context", func() {

				err := au.Write(ctx, approved.UserInfo{UserID: "789", UserName: "test"})
				s.Require().NoError(err)

				ctxCanceled, cancel := context.WithCancel(context.Background())
				cancel()

				err = au.Delete(ctxCanceled, "789")
				s.Require().Error(err)
				s.Contains(err.Error(), "context canceled")
			})

			s.Run("context timeout", func() {
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
				defer cancel()
				time.Sleep(time.Millisecond)

				err := au.Write(ctxTimeout, approved.UserInfo{UserID: "timeout", UserName: "test"})
				s.Require().Error(err)
				s.Contains(err.Error(), "context deadline exceeded")
			})
		})
	}
}

func (s *StorageTestSuite) TestApprovedUsers_ErrorCases() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			au, err := NewApprovedUsers(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE approved_users")

			clearDB := func() {
				_, err := db.Exec(db.Adopt("DELETE FROM approved_users"))
				s.Require().NoError(err)
			}

			s.Run("empty user id", func() {
				clearDB()
				user := approved.UserInfo{
					UserName: "test",
				}
				err := au.Write(ctx, user)
				s.Require().Error(err)
				s.Equal("user id can't be empty", err.Error())
			})

			s.Run("empty username should be valid", func() {
				clearDB()
				user := approved.UserInfo{
					UserID: "123",
				}
				err := au.Write(ctx, user)
				s.Require().NoError(err)
			})

			s.Run("delete non-existent", func() {
				clearDB()
				err := au.Delete(ctx, "non-existent")
				s.Require().Error(err)
				s.Contains(err.Error(), "failed to get approved user")
			})

			s.Run("delete empty id", func() {
				clearDB()
				err := au.Delete(ctx, "")
				s.Require().Error(err)
				s.Equal("user id can't be empty", err.Error())
			})

			s.Run("concurrent write same user", func() {
				clearDB()
				user := approved.UserInfo{
					UserID:    "456",
					UserName:  "test",
					Timestamp: time.Now(),
				}

				var wg sync.WaitGroup
				errCh := make(chan error, 2)

				for range 2 {
					wg.Go(func() {
						if err := au.Write(ctx, user); err != nil {
							errCh <- err
						}
					})
				}

				wg.Wait()
				close(errCh)

				// collect any errors
				var errs []error
				for err := range errCh {
					errs = append(errs, err)
				}
				s.Empty(errs)

				users, err := au.Read(ctx)
				s.Require().NoError(err)
				s.Require().Len(users, 1, "should only have one user record")
				s.Equal(user.UserID, users[0].UserID)
				s.Equal(user.UserName, users[0].UserName)
			})
		})
	}
}
