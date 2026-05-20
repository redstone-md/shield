package storage

import (
	"bufio"
	"context"
	"fmt"
	"github.com/umputun/tg-spam/app/storage/engine"
	"strings"
	"sync"
	"time"
)

func (s *StorageTestSuite) TestSamples_Concurrent() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {

			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			s.Require().NotNil(samples)
			defer db.Exec("DROP TABLE samples")

			err = samples.Add(ctx, SampleTypeHam, SampleOriginPreset, "test message")
			s.Require().NoError(err, "Failed to insert initial test record")

			const numWorkers = 10
			const numOps = 50

			var wg sync.WaitGroup
			errCh := make(chan error, numWorkers*2)

			for i := range numWorkers {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					for range numOps {
						if _, readErr := samples.Read(ctx, SampleTypeHam, SampleOriginAny); readErr != nil {
							select {
							case errCh <- fmt.Errorf("reader %d failed: %w", workerID, readErr):
							default:
							}
							return
						}
					}
				}(i)
			}

			for i := range numWorkers {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					for j := range numOps {
						msg := fmt.Sprintf("test message %d-%d", workerID, j)
						sType := SampleTypeHam
						if j%2 == 0 {
							sType = SampleTypeSpam
						}
						if addErr := samples.Add(ctx, sType, SampleOriginUser, msg); addErr != nil {
							select {
							case errCh <- fmt.Errorf("writer %d failed: %w", workerID, addErr):
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
				s.T().Errorf("concurrent operation failed: %v", err)
			}

			stats, err := samples.Stats(ctx)
			s.Require().NoError(err)
			s.Require().NotNil(stats)

			expectedTotal := numWorkers*numOps + 1
			actualTotal := stats.TotalHam + stats.TotalSpam
			s.Equal(expectedTotal, actualTotal, "expected %d total samples, got %d", expectedTotal, actualTotal)
		})
	}
}

func (s *StorageTestSuite) TestSamples_Iterator() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			testData := []struct {
				sType   SampleType
				origin  SampleOrigin
				message string
			}{
				{SampleTypeHam, SampleOriginPreset, "ham preset 1"},
				{SampleTypeHam, SampleOriginUser, "ham user 1"},
				{SampleTypeSpam, SampleOriginPreset, "spam preset 1"},
				{SampleTypeSpam, SampleOriginUser, "spam user 1"},
			}

			for _, td := range testData {
				err := samples.Add(ctx, td.sType, td.origin, td.message)
				s.Require().NoError(err)
			}

			tests := []struct {
				name         string
				sType        SampleType
				origin       SampleOrigin
				expectedMsgs []string
				expectErr    bool
			}{
				{
					name:         "Ham Preset Samples",
					sType:        SampleTypeHam,
					origin:       SampleOriginPreset,
					expectedMsgs: []string{"ham preset 1"},
					expectErr:    false,
				},
				{
					name:         "Spam User Samples",
					sType:        SampleTypeSpam,
					origin:       SampleOriginUser,
					expectedMsgs: []string{"spam user 1"},
					expectErr:    false,
				},
				{
					name:         "All Ham Samples",
					sType:        SampleTypeHam,
					origin:       SampleOriginAny,
					expectedMsgs: []string{"ham preset 1", "ham user 1"},
					expectErr:    false,
				},
				{
					name:         "Invalid Sample Type",
					sType:        "invalid",
					origin:       SampleOriginPreset,
					expectedMsgs: nil,
					expectErr:    true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					iter, err := samples.Iterator(ctx, tt.sType, tt.origin)
					if tt.expectErr {
						s.Error(err)
						return
					}
					s.Require().NoError(err)

					var messages []string
					for msg := range iter {
						messages = append(messages, msg)
					}

					s.ElementsMatch(tt.expectedMsgs, messages)
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_IteratorOrder() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			testData := []struct {
				sType   SampleType
				origin  SampleOrigin
				message string
			}{
				{SampleTypeHam, SampleOriginPreset, "ham preset 1"},
				{SampleTypeHam, SampleOriginPreset, "ham preset 2"},
				{SampleTypeHam, SampleOriginPreset, "ham preset 3"},
			}

			for _, td := range testData {
				addErr := samples.Add(ctx, td.sType, td.origin, td.message)
				s.Require().NoError(addErr)
				time.Sleep(time.Second)
			}

			iter, err := samples.Iterator(ctx, SampleTypeHam, SampleOriginPreset)
			s.Require().NoError(err)
			messages := make([]string, 0, 3)
			for msg := range iter {
				messages = append(messages, msg)
			}
			s.Require().Len(messages, 3)
			s.Equal("ham preset 3", messages[0])
			s.Equal("ham preset 2", messages[1])
			s.Equal("ham preset 1", messages[2])
		})
	}
}

func (s *StorageTestSuite) TestSamples_Import() {
	ctx := context.Background()

	countSamples := func(db *engine.SQL, t SampleType, o SampleOrigin) int {
		var count int
		err := db.Get(&count, db.Adopt("SELECT COUNT(*) FROM samples WHERE type = ? AND origin = ?"), t, o)
		if err != nil {
			return -1
		}
		return count
	}

	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			s.Run("basic import with cleanup", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("sample1\nsample2\nsample3")
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginPreset, input, true)
				s.Require().NoError(err)
				s.Require().NotNil(stats)

				s.Equal(3, countSamples(db, SampleTypeHam, SampleOriginPreset))
				s.Equal(3, stats.PresetHam)
			})

			s.Run("import without cleanup should append", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input1 := strings.NewReader("existing1\nexisting2")
				_, err = samples.Import(ctx, SampleTypeSpam, SampleOriginPreset, input1, true)
				s.Require().NoError(err)
				s.Equal(2, countSamples(db, SampleTypeSpam, SampleOriginPreset))

				input2 := strings.NewReader("new1\nnew2")
				stats, err := samples.Import(ctx, SampleTypeSpam, SampleOriginPreset, input2, false)
				s.Require().NoError(err)
				s.Require().NotNil(stats)

				s.Equal(4, countSamples(db, SampleTypeSpam, SampleOriginPreset))
				s.Equal(4, stats.PresetSpam)

				res, err := samples.Read(ctx, SampleTypeSpam, SampleOriginPreset)
				s.Require().NoError(err)
				s.ElementsMatch([]string{"existing1", "existing2", "new1", "new2"}, res)
			})

			s.Run("import with cleanup should replace", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input1 := strings.NewReader("old1\nold2\nold3")
				_, err = samples.Import(ctx, SampleTypeSpam, SampleOriginUser, input1, true)
				s.Require().NoError(err)
				s.Equal(3, countSamples(db, SampleTypeSpam, SampleOriginUser))

				input2 := strings.NewReader("new1\nnew2")
				stats, err := samples.Import(ctx, SampleTypeSpam, SampleOriginUser, input2, true)
				s.Require().NoError(err)
				s.Require().NotNil(stats)

				s.Equal(2, countSamples(db, SampleTypeSpam, SampleOriginUser))
				s.Equal(2, stats.UserSpam)

				res, err := samples.Read(ctx, SampleTypeSpam, SampleOriginUser)
				s.Require().NoError(err)
				s.ElementsMatch([]string{"new1", "new2"}, res)
			})

			s.Run("different types preserve independence", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				inputHam := strings.NewReader("ham1\nham2")
				_, err = samples.Import(ctx, SampleTypeHam, SampleOriginUser, inputHam, true)
				s.Require().NoError(err)

				inputSpam := strings.NewReader("spam1\nspam2\nspam3")
				stats, err := samples.Import(ctx, SampleTypeSpam, SampleOriginUser, inputSpam, true)
				s.Require().NoError(err)
				s.Require().NotNil(stats)

				s.Equal(2, countSamples(db, SampleTypeHam, SampleOriginUser))
				s.Equal(3, countSamples(db, SampleTypeSpam, SampleOriginUser))
			})

			s.Run("invalid type", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("sample")
				_, err = samples.Import(ctx, "invalid", SampleOriginPreset, input, true)
				s.Error(err)
			})

			s.Run("invalid origin", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("sample")
				_, err = samples.Import(ctx, SampleTypeHam, "invalid", input, true)
				s.Error(err)
			})

			s.Run("origin any not allowed", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("sample")
				_, err = samples.Import(ctx, SampleTypeHam, SampleOriginAny, input, true)
				s.Error(err)
			})

			s.Run("empty input", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("")
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginPreset, input, true)
				s.Require().NoError(err)
				s.Require().NotNil(stats)
				s.Equal(0, countSamples(db, SampleTypeHam, SampleOriginPreset))
			})

			s.Run("input with empty lines", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				input := strings.NewReader("sample1\n\n\nsample2\n\n")
				stats, err := samples.Import(ctx, SampleTypeHam, SampleOriginPreset, input, true)
				s.Require().NoError(err)
				s.Require().NotNil(stats)

				res, err := samples.Read(ctx, SampleTypeHam, SampleOriginPreset)
				s.Require().NoError(err)
				s.ElementsMatch([]string{"sample1", "sample2"}, res)
			})

			s.Run("nil reader", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				_, err = samples.Import(ctx, SampleTypeHam, SampleOriginPreset, nil, true)
				s.Error(err)
			})

			s.Run("reader error", func() {
				samples, err := NewSamples(ctx, db)
				s.Require().NoError(err)
				defer db.Exec("DROP TABLE samples")

				errReader := &errorReader{err: fmt.Errorf("read error")}
				_, err = samples.Import(ctx, SampleTypeHam, SampleOriginPreset, errReader, true)
				s.Error(err)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_Reader() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			tests := []struct {
				name       string
				setup      func(*Samples)
				sampleType SampleType
				origin     SampleOrigin
				want       []string
				wantErr    bool
			}{
				{
					name: "ham samples",
					setup: func(sm *Samples) {
						s.Require().NoError(sm.Add(context.Background(), SampleTypeHam, SampleOriginPreset, "test1"))
						time.Sleep(time.Second)
						s.Require().NoError(sm.Add(context.Background(), SampleTypeHam, SampleOriginPreset, "test2"))
					},
					sampleType: SampleTypeHam,
					origin:     SampleOriginPreset,
					want:       []string{"test2", "test1"},
				},
				{
					name: "empty result",
					setup: func(s *Samples) {

					},
					sampleType: SampleTypeSpam,
					origin:     SampleOriginUser,
					want:       []string(nil),
				},
				{
					name:       "invalid type",
					sampleType: "invalid",
					origin:     SampleOriginPreset,
					wantErr:    true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					samples, err := NewSamples(ctx, db)
					s.Require().NoError(err)
					defer db.Exec("DROP TABLE samples")

					if tt.setup != nil {
						tt.setup(samples)
					}

					r, err := samples.Reader(ctx, tt.sampleType, tt.origin)
					if tt.wantErr {
						s.Error(err)
						return
					}
					s.Require().NoError(err)

					lines := 0
					scanner := bufio.NewScanner(r)
					var got []string
					for scanner.Scan() {
						lines++
						got = append(got, scanner.Text())
					}
					s.Require().NoError(scanner.Err())
					s.Equal(tt.want, got)
					s.Equal(len(tt.want), lines)

					s.NoError(r.Close())
				})
			}
		})
	}
}
