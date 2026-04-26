package storage

import (
	"context"
	"fmt"
	"github.com/umputun/tg-spam/app/storage/engine"
	"strings"
	"sync"
)

func (s *StorageTestSuite) TestNewSamples() {
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			defer db.Exec("DROP TABLE samples")

			tests := []struct {
				name    string
				db      *engine.SQL
				wantErr bool
			}{
				{
					name:    "valid db connection",
					db:      db,
					wantErr: false,
				},
				{
					name:    "nil db connection",
					db:      nil,
					wantErr: true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					samples, err := NewSamples(context.Background(), tt.db)
					if tt.wantErr {
						s.Require().Error(err)
						s.Nil(samples)
					} else {
						s.Require().NoError(err)
						s.NotNil(samples)
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_AddSample() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			tests := []struct {
				name    string
				sType   SampleType
				origin  SampleOrigin
				message string
				wantErr bool
			}{
				{
					name:    "valid ham preset",
					sType:   SampleTypeHam,
					origin:  SampleOriginPreset,
					message: "test ham message",
					wantErr: false,
				},
				{
					name:    "valid spam user",
					sType:   SampleTypeSpam,
					origin:  SampleOriginUser,
					message: "test spam message",
					wantErr: false,
				},
				{
					name:    "invalid sample type",
					sType:   "invalid",
					origin:  SampleOriginPreset,
					message: "test message",
					wantErr: true,
				},
				{
					name:    "invalid origin",
					sType:   SampleTypeHam,
					origin:  "invalid",
					message: "test message",
					wantErr: true,
				},
				{
					name:    "empty message",
					sType:   SampleTypeHam,
					origin:  SampleOriginPreset,
					message: "",
					wantErr: true,
				},
				{
					name:    "origin any not allowed",
					sType:   SampleTypeHam,
					origin:  SampleOriginAny,
					message: "test message",
					wantErr: true,
				},
				{
					name:    "duplicate message same type and origin",
					sType:   SampleTypeHam,
					origin:  SampleOriginPreset,
					message: "test ham message",
					wantErr: false,
				},
				{
					name:    "duplicate message different type",
					sType:   SampleTypeSpam,
					origin:  SampleOriginPreset,
					message: "test ham message",
					wantErr: false,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					err := samples.Add(ctx, tt.sType, tt.origin, tt.message)
					if tt.wantErr {
						s.Require().Error(err)
					} else {
						s.Require().NoError(err)
						// verify message exists and has correct type and origin
						var count int
						err = db.Get(&count, db.Adopt("SELECT COUNT(*) FROM samples WHERE message = ? AND type = ? AND origin = ?"),
							tt.message, tt.sType, tt.origin)
						s.Require().NoError(err)
						s.Equal(1, count)
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_DeleteSample() {
	ctx := context.Background()
	for _, dbt := range s.getTestDB() {
		db := dbt.DB
		s.Run(fmt.Sprintf("with %s", db.Type()), func() {
			samples, err := NewSamples(ctx, db)
			s.Require().NoError(err)
			defer db.Exec("DROP TABLE samples")

			err = samples.Add(ctx, SampleTypeHam, SampleOriginPreset, "test message")
			s.Require().NoError(err)

			// get the ID of the inserted sample
			var id int64
			err = db.Get(&id, db.Adopt("SELECT id FROM samples WHERE message = ?"), "test message")
			s.Require().NoError(err)

			tests := []struct {
				name    string
				id      int64
				wantErr bool
			}{
				{
					name:    "existing sample",
					id:      id,
					wantErr: false,
				},
				{
					name:    "non-existent sample",
					id:      99999,
					wantErr: true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					err := samples.Delete(ctx, tt.id)
					if tt.wantErr {
						s.Error(err)
					} else {
						s.NoError(err)
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_DeleteMessage() {
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
				{SampleTypeHam, SampleOriginPreset, "message to delete"},
				{SampleTypeSpam, SampleOriginUser, "message to keep"},
				{SampleTypeHam, SampleOriginUser, "another message"},
			}

			for _, td := range testData {
				addErr := samples.Add(ctx, td.sType, td.origin, td.message)
				s.Require().NoError(addErr)
			}

			tests := []struct {
				name    string
				message string
				wantErr bool
			}{
				{
					name:    "existing message",
					message: "message to delete",
					wantErr: false,
				},
				{
					name:    "non-existent message",
					message: "no such message",
					wantErr: true,
				},
				{
					name:    "empty message",
					message: "",
					wantErr: true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					delErr := samples.DeleteMessage(ctx, tt.message)
					if tt.wantErr {
						s.Require().Error(delErr)
					} else {
						s.Require().NoError(delErr)

						// verify message no longer exists
						var count int
						err = db.Get(&count, db.Adopt("SELECT COUNT(*) FROM samples WHERE message = ?"), tt.message)
						s.Require().NoError(err)
						s.Equal(0, count)

						// verify other messages still exist
						var totalCount int
						err = db.Get(&totalCount, db.Adopt("SELECT COUNT(*) FROM samples"))
						s.Require().NoError(err)
						s.Equal(len(testData)-1, totalCount)
					}
				})
			}

			s.Run("concurrent delete", func() {

				baseMsg := "concurrent delete message"
				for i := range 10 {
					addErr := samples.Add(ctx, SampleTypeHam, SampleOriginPreset, baseMsg+fmt.Sprintf("-%d", i))
					s.Require().NoError(addErr)
				}

				const numWorkers = 10
				var wg sync.WaitGroup
				errCh := make(chan error, numWorkers)

				for i := range numWorkers {
					wg.Add(1)
					go func(idx int) {
						defer wg.Done()
						delMsg := baseMsg + fmt.Sprintf("-%d", idx)
						if delErr := samples.DeleteMessage(ctx, delMsg); delErr != nil && !strings.Contains(delErr.Error(), "not found") {
							errCh <- delErr
						}
					}(i)
				}

				wg.Wait()
				close(errCh)

				for err := range errCh {
					s.T().Errorf("concurrent delete failed: %v", err)
				}

				// verify message was deleted
				var count int
				err = db.Get(&count, db.Adopt("SELECT COUNT(*) FROM samples WHERE message = ?"), baseMsg)
				s.Require().NoError(err)
				s.Equal(0, count)
			})
		})
	}
}

func (s *StorageTestSuite) TestSamples_ReadSamples() {
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
				name          string
				sType         SampleType
				origin        SampleOrigin
				expectedCount int
				wantErr       bool
			}{
				{
					name:          "all ham samples",
					sType:         SampleTypeHam,
					origin:        SampleOriginAny,
					expectedCount: 2,
					wantErr:       false,
				},
				{
					name:          "preset spam samples",
					sType:         SampleTypeSpam,
					origin:        SampleOriginPreset,
					expectedCount: 1,
					wantErr:       false,
				},
				{
					name:          "invalid type",
					sType:         "invalid",
					origin:        SampleOriginPreset,
					expectedCount: 0,
					wantErr:       true,
				},
				{
					name:          "invalid origin",
					sType:         SampleTypeHam,
					origin:        "invalid",
					expectedCount: 0,
					wantErr:       true,
				},
			}

			for _, tt := range tests {
				s.Run(tt.name, func() {
					samples, err := samples.Read(ctx, tt.sType, tt.origin)
					if tt.wantErr {
						s.Require().Error(err)
						s.Nil(samples)
					} else {
						s.Require().NoError(err)
						s.Len(samples, tt.expectedCount)
					}
				})
			}
		})
	}
}

func (s *StorageTestSuite) TestSamples_GetStats() {
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
				{SampleTypeHam, SampleOriginUser, "ham user 1"},
				{SampleTypeSpam, SampleOriginPreset, "spam preset 1"},
				{SampleTypeSpam, SampleOriginUser, "spam user 1"},
				{SampleTypeSpam, SampleOriginUser, "spam user 2"},
			}

			for _, td := range testData {
				addErr := samples.Add(ctx, td.sType, td.origin, td.message)
				s.Require().NoError(addErr)
			}

			stats, err := samples.Stats(ctx)
			s.Require().NoError(err)
			s.Require().NotNil(stats)

			s.Equal(3, stats.TotalSpam)
			s.Equal(3, stats.TotalHam)
			s.Equal(1, stats.PresetSpam)
			s.Equal(2, stats.PresetHam)
			s.Equal(2, stats.UserSpam)
			s.Equal(1, stats.UserHam)
		})
	}
}

func (s *StorageTestSuite) TestSampleType_Validate() {
	tests := []struct {
		name    string
		sType   SampleType
		wantErr bool
	}{
		{"valid ham", SampleTypeHam, false},
		{"valid spam", SampleTypeSpam, false},
		{"invalid type", "invalid", true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.sType.Validate()
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *StorageTestSuite) TestSampleOrigin_Validate() {
	tests := []struct {
		name    string
		origin  SampleOrigin
		wantErr bool
	}{
		{"valid preset", SampleOriginPreset, false},
		{"valid user", SampleOriginUser, false},
		{"valid any", SampleOriginAny, false},
		{"invalid origin", "invalid", true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.origin.Validate()
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}
