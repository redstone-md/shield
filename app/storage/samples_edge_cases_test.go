package storage

import (
	"context"
	"fmt"
	"github.com/umputun/tg-spam/app/storage/engine"
	"io"
	"strings"
	"time"
)

func (s *StorageTestSuite) TestSamples_ReaderEdgeCases() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			s.Run("large message handling", func() {

				msgSize := 1024 * 1024
				if db.Type() == engine.Postgres {
					msgSize = 4096
				}
				largeMsg := strings.Repeat("a", msgSize)
				err := samples.Add(ctx, SampleTypeHam, SampleOriginUser, largeMsg)
				s.Require().NoError(err)

				reader, err := samples.Reader(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)
				defer reader.Close()

				buf := make([]byte, 1024)
				total := 0
				for {
					n, err := reader.Read(buf)
					total += n
					if err == io.EOF {
						break
					}
					s.Require().NoError(err)
				}
				s.Equal(len(largeMsg)+1, total)
			})

			s.Run("multiple close calls", func() {
				reader, err := samples.Reader(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)

				s.Require().NoError(reader.Close())
				s.Require().NoError(reader.Close())
				s.Require().NoError(reader.Close())
			})

			s.Run("read after close", func() {
				reader, err := samples.Reader(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)

				s.Require().NoError(reader.Close())

				buf := make([]byte, 1024)
				n, err := reader.Read(buf)
				s.Equal(0, n)
				s.Error(err)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_ImportEdgeCases() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			s.Run("import very long lines", func() {

				longLine := strings.Repeat("a", 64*1024)
				input := strings.NewReader(longLine)
				_, err := samples.Import(ctx, SampleTypeHam, SampleOriginUser, input, true)
				s.Require().Error(err)
				s.Contains(err.Error(), "token too long")
			})

			s.Run("64k-1 line should succeed", func() {
				longLine := strings.Repeat("a", 64*1024-1)
				input := strings.NewReader(longLine)
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginUser, input, true)
				s.Require().NoError(err)
				s.Equal(1, stats.UserHam)
			})

			s.Run("import with unicode", func() {
				unicodeText := "привет\n你好\nこんにちは\n"
				input := strings.NewReader(unicodeText)
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginUser, input, true)
				s.Require().NoError(err)
				s.Equal(3, stats.UserHam)

				samples, err := samples.Read(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)
				s.Contains(samples, "привет")
				s.Contains(samples, "你好")
				s.Contains(samples, "こんにちは")
			})

			s.Run("zero byte reader", func() {
				input := strings.NewReader("")
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginUser, input, true)
				s.Require().NoError(err)
				s.Equal(0, stats.UserHam)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_IteratorBehavior() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			s.Run("early termination", func() {

				for i := range 10 {
					addErr := samples.Add(ctx, SampleTypeHam, SampleOriginUser, fmt.Sprintf("msg%d", i))
					s.Require().NoError(addErr)
				}

				iter, err := samples.Iterator(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)

				count := 0
				for msg := range iter {
					count++
					if count == 5 {
						break
					}
					_ = msg
				}
				s.Equal(5, count)
			})

			s.Run("context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())
				iter, err := samples.Iterator(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)

				count := 0
				done := make(chan bool)
				go func() {
					for msg := range iter {
						count++
						if count == 2 {
							cancel()
						}
						_ = msg
					}
					done <- true
				}()

				select {
				case <-done:
					s.Less(count, 10)
				case <-time.After(time.Second):
					s.Fail("iterator did not terminate after context cancellation")
				}
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_Validation() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			s.Run("unicode sample type validation", func() {
				err := samples.Add(ctx, SampleType("спам"), SampleOriginUser, "test")
				s.Error(err)
			})

			s.Run("unicode origin validation", func() {
				err := samples.Add(ctx, SampleTypeHam, SampleOrigin("用户"), "test")
				s.Error(err)
			})

			s.Run("emoji in message", func() {
				msg := "test 👍 message 🚀"
				err := samples.Add(ctx, SampleTypeHam, SampleOriginUser, msg)
				s.Require().NoError(err)

				samples, err := samples.Read(ctx, SampleTypeHam, SampleOriginUser)
				s.Require().NoError(err)
				s.Contains(samples, msg)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_DatabaseErrors() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			s.Run("transaction rollback", func() {

				_, err := db.Exec("DROP TABLE samples")
				s.Require().NoError(err)

				err = samples.Add(ctx, SampleTypeHam, SampleOriginUser, "test")
				s.Error(err)
			})

			s.Run("invalid sql", func() {

				invalidSQL := "CREATE TABLE samples (invalid) xyz"
				if db.Type() == engine.Postgres {
					invalidSQL = "CREATE TABLE samples (id INTEGER, CONSTRAINT bad_const CHECK (id > 'text'))"
				}
				_, err := db.Exec(invalidSQL)
				s.Require().Error(err)

				stats, err := samples.Stats(ctx)
				s.Require().Error(err)
				s.Nil(stats)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_ImportSizeLimits() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			sm, err := NewSamples(ctx, db)
			s.Require().NoError(err)

			tests := []struct {
				name     string
				input    string
				wantErr  bool
				expected int // expected number of samples
			}{
				{
					name:     "small lines",
					input:    "short line 1\nshort line 2\n",
					wantErr:  false,
					expected: 2,
				},
				{
					name:     "64k-1 line",
					input:    strings.Repeat("a", 64*1024-1) + "\n",
					wantErr:  false,
					expected: 1,
				},
				{
					name:     "64k line fails by default",
					input:    strings.Repeat("a", 64*1024) + "\n",
					wantErr:  true,
					expected: 1,
				},
				{
					name:     "1MB line fails by default",
					input:    strings.Repeat("a", 1024*1024) + "\n",
					wantErr:  true,
					expected: 0,
				},
				{
					name:     "empty lines",
					input:    "\n\n\n",
					wantErr:  false,
					expected: 0,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					stats, err := sm.Import(ctx, SampleTypeHam, SampleOriginUser, strings.NewReader(tt.input), true)
					if tt.wantErr {
						s.Error(err)
						return
					}
					s.Require().NoError(err)
					s.Equal(tt.expected, stats.UserHam)

					if !tt.wantErr {
						samples, err := sm.Read(ctx, SampleTypeHam, SampleOriginUser)
						s.Require().NoError(err)
						s.Len(samples, tt.expected)

						for line := range strings.SplitSeq(strings.TrimSpace(tt.input), "\n") {
							if line != "" {
								s.Contains(samples, line)
							}
						}
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_ReaderUnlock() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			sm, err := NewSamples(ctx, db)
			s.Require().NoError(err)

			err = sm.Add(ctx, SampleTypeSpam, SampleOriginUser, "test message 1")
			s.Require().NoError(err)

			r1, err := sm.Reader(ctx, SampleTypeSpam, SampleOriginUser)
			s.Require().NoError(err)
			data, err := io.ReadAll(r1)
			s.Require().NoError(err)
			s.NotEmpty(data)
			r1.Close()

			err = sm.Add(ctx, SampleTypeSpam, SampleOriginUser, "test message 2")
			s.Require().NoError(err)

			r2, err := sm.Reader(ctx, SampleTypeSpam, SampleOriginUser)
			s.Require().NoError(err)
			data, err = io.ReadAll(r2)
			s.Require().NoError(err)
			s.NotEmpty(data)
			r2.Close()
		})
	}
}

func (s *StorageTestSuite) TestSamples_LongMessages() {
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE IF EXISTS samples")
			samples, err := NewSamples(context.Background(), db)
			s.Require().NoError(err)

			tests := []struct {
				name    string
				message string
			}{
				{
					name:    "small message",
					message: "hello world",
				},
				{
					name:    "medium message",
					message: strings.Repeat("x", 1000),
				},
				{
					name:    "large message",
					message: strings.Repeat("y", 10000),
				},
				{
					name:    "very large message",
					message: strings.Repeat("z", 100000),
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {

					err := samples.Add(context.Background(), SampleTypeSpam, SampleOriginUser, tt.message)
					s.Require().NoError(err, "should add message")

					msgs, err := samples.Read(context.Background(), SampleTypeSpam, SampleOriginUser)
					s.Require().NoError(err)
					s.Require().Contains(msgs, tt.message)

					err = samples.DeleteMessage(context.Background(), tt.message)
					s.Require().NoError(err, "should delete message")

					msgs, err = samples.Read(context.Background(), SampleTypeSpam, SampleOriginUser)
					s.Require().NoError(err)
					s.Require().NotContains(msgs, tt.message)
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_DuplicateLongMessages() {
	longMsg := strings.Repeat("x", 50000)

	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE IF EXISTS samples")
			samples, err := NewSamples(context.Background(), db)
			s.Require().NoError(err)

			for i := range 3 {
				addErr := samples.Add(context.Background(), SampleTypeSpam, SampleOriginUser, longMsg)
				s.Require().NoError(addErr, "should add message attempt %d", i)
			}

			msgs, err := samples.Read(context.Background(), SampleTypeSpam, SampleOriginUser)
			s.Require().NoError(err)
			count := 0
			for _, msg := range msgs {
				if msg == longMsg {
					count++
				}
			}
			s.Equal(1, count, "should have only one instance of the message")
		})
	}
}

// errorReader implements io.Reader interface and always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (n int, err error) {
	return 0, r.err
}
