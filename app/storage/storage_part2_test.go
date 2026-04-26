package storage

import (
	"context"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func (s *StorageTestSuite) TestIsolation() {
	ctx := context.Background()
	s.Run("sqlite isolation via separate files", func() {

		tmpFile1 := filepath.Join(os.TempDir(), "test_db1.sqlite")
		tmpFile2 := filepath.Join(os.TempDir(), "test_db2.sqlite")
		defer os.Remove(tmpFile1)
		defer os.Remove(tmpFile2)

		db1, err := engine.NewSqlite(tmpFile1, "gr1")
		s.Require().NoError(err)
		defer db1.Close()

		db2, err := engine.NewSqlite(tmpFile2, "gr2")
		s.Require().NoError(err)
		defer db2.Close()

		s.Run("samples isolation", func() {
			samples1, err := NewSamples(ctx, db1)
			s.Require().NoError(err)
			samples2, err := NewSamples(ctx, db2)
			s.Require().NoError(err)

			err = samples1.Add(ctx, SampleTypeHam, SampleOriginUser, "test1 message")
			s.Require().NoError(err)
			err = samples2.Add(ctx, SampleTypeHam, SampleOriginUser, "test2 message")
			s.Require().NoError(err)

			msgs1, err := samples1.Read(ctx, SampleTypeHam, SampleOriginUser)
			s.Require().NoError(err)
			s.Len(msgs1, 1)
			s.Equal("test1 message", msgs1[0])

			msgs2, err := samples2.Read(ctx, SampleTypeHam, SampleOriginUser)
			s.Require().NoError(err)
			s.Len(msgs2, 1)
			s.Equal("test2 message", msgs2[0])
		})

		s.Run("dictionary isolation", func() {
			dict1, err := NewDictionary(ctx, db1)
			s.Require().NoError(err)
			dict2, err := NewDictionary(ctx, db2)
			s.Require().NoError(err)

			err = dict1.Add(ctx, DictionaryTypeStopPhrase, "test1 phrase")
			s.Require().NoError(err)
			err = dict2.Add(ctx, DictionaryTypeStopPhrase, "test2 phrase")
			s.Require().NoError(err)

			phrases1, err := dict1.Read(ctx, DictionaryTypeStopPhrase)
			s.Require().NoError(err)
			s.Len(phrases1, 1)
			s.Equal("test1 phrase", phrases1[0])

			phrases2, err := dict2.Read(ctx, DictionaryTypeStopPhrase)
			s.Require().NoError(err)
			s.Len(phrases2, 1)
			s.Equal("test2 phrase", phrases2[0])
		})

		s.Run("detected spam isolation", func() {
			spam1, err := NewDetectedSpam(ctx, db1)
			s.Require().NoError(err)
			spam2, err := NewDetectedSpam(ctx, db2)
			s.Require().NoError(err)

			entry1 := DetectedSpamInfo{
				GID:       "gr1",
				Text:      "spam1",
				UserID:    1,
				UserName:  "user1",
				Timestamp: time.Now(),
			}
			entry2 := DetectedSpamInfo{
				GID:       "gr2",
				Text:      "spam2",
				UserID:    2,
				UserName:  "user2",
				Timestamp: time.Now(),
			}
			checks := []spamcheck.Response{{Name: "test", Spam: true}}

			err = spam1.Write(ctx, entry1, checks)
			s.Require().NoError(err)
			err = spam2.Write(ctx, entry2, checks)
			s.Require().NoError(err)

			entries1, err := spam1.Read(ctx)
			s.Require().NoError(err)
			s.Len(entries1, 1)
			s.Equal("spam1", entries1[0].Text)

			entries2, err := spam2.Read(ctx)
			s.Require().NoError(err)
			s.Len(entries2, 1)
			s.Equal("spam2", entries2[0].Text)
		})

		s.Run("approved users isolation", func() {
			au1, err := NewApprovedUsers(ctx, db1)
			s.Require().NoError(err)
			au2, err := NewApprovedUsers(ctx, db2)
			s.Require().NoError(err)

			user1 := approved.UserInfo{UserID: "1", UserName: "user1"}
			user2 := approved.UserInfo{UserID: "2", UserName: "user2"}

			err = au1.Write(ctx, user1)
			s.Require().NoError(err)
			err = au2.Write(ctx, user2)
			s.Require().NoError(err)

			users1, err := au1.Read(ctx)
			s.Require().NoError(err)
			s.Len(users1, 1)
			s.Equal("user1", users1[0].UserName)

			users2, err := au2.Read(ctx)
			s.Require().NoError(err)
			s.Len(users2, 1)
			s.Equal("user2", users2[0].UserName)
		})

		s.Run("locator isolation", func() {
			loc1, err := NewLocator(ctx, time.Hour, 10, db1)
			s.Require().NoError(err)
			loc2, err := NewLocator(ctx, time.Hour, 10, db2)
			s.Require().NoError(err)

			err = loc1.AddMessage(ctx, "msg1", 1, 1, "user1", 1)
			s.Require().NoError(err)
			err = loc2.AddMessage(ctx, "msg2", 2, 2, "user2", 2)
			s.Require().NoError(err)

			meta1, found := loc1.Message(ctx, "msg1")
			s.Require().True(found)
			s.Equal(int64(1), meta1.UserID)

			meta2, found := loc2.Message(ctx, "msg2")
			s.Require().True(found)
			s.Equal(int64(2), meta2.UserID)

			_, found = loc1.Message(ctx, "msg2")
			s.Require().False(found)
			_, found = loc2.Message(ctx, "msg1")
			s.Require().False(found)
		})
	})

	s.Run("postgres isolation via gid column", func() {

		if testing.Short() {
			s.T().Skip("skipping postgres test in short mode")
		}

		// get existing postgres container connection
		var pgDB *engine.SQL
		for _, dbt := range s.getTestDB() {
			if dbt.DB.Type() == engine.Postgres {
				pgDB = dbt.DB
				break
			}
		}
		if pgDB == nil {
			s.T().Skip("postgres is not available")
		}

		pgConnStr := s.pgContainer.ConnectionString()

		db1, err := engine.NewPostgres(ctx, pgConnStr, "gr1")
		s.Require().NoError(err)
		defer db1.Close()

		db2, err := engine.NewPostgres(ctx, pgConnStr, "gr2")
		s.Require().NoError(err)
		defer db2.Close()

		s.Run("samples isolation", func() {
			samples1, err := NewSamples(ctx, db1)
			s.Require().NoError(err)
			samples2, err := NewSamples(ctx, db2)
			s.Require().NoError(err)

			err = samples1.Add(ctx, SampleTypeHam, SampleOriginUser, "test1 message")
			s.Require().NoError(err)
			err = samples2.Add(ctx, SampleTypeHam, SampleOriginUser, "test2 message")
			s.Require().NoError(err)

			msgs1, err := samples1.Read(ctx, SampleTypeHam, SampleOriginUser)
			s.Require().NoError(err)
			s.Len(msgs1, 1)
			s.Equal("test1 message", msgs1[0])

			msgs2, err := samples2.Read(ctx, SampleTypeHam, SampleOriginUser)
			s.Require().NoError(err)
			s.Len(msgs2, 1)
			s.Equal("test2 message", msgs2[0])
		})

		s.Run("dictionary isolation", func() {
			dict1, err := NewDictionary(ctx, db1)
			s.Require().NoError(err)
			dict2, err := NewDictionary(ctx, db2)
			s.Require().NoError(err)

			err = dict1.Add(ctx, DictionaryTypeStopPhrase, "test1 phrase")
			s.Require().NoError(err)
			err = dict2.Add(ctx, DictionaryTypeStopPhrase, "test2 phrase")
			s.Require().NoError(err)

			phrases1, err := dict1.Read(ctx, DictionaryTypeStopPhrase)
			s.Require().NoError(err)
			s.Len(phrases1, 1)
			s.Equal("test1 phrase", phrases1[0])

			phrases2, err := dict2.Read(ctx, DictionaryTypeStopPhrase)
			s.Require().NoError(err)
			s.Len(phrases2, 1)
			s.Equal("test2 phrase", phrases2[0])
		})

		s.Run("detected spam isolation", func() {
			spam1, err := NewDetectedSpam(ctx, db1)
			s.Require().NoError(err)
			spam2, err := NewDetectedSpam(ctx, db2)
			s.Require().NoError(err)

			entry1 := DetectedSpamInfo{
				GID:       "gr1",
				Text:      "spam1",
				UserID:    1,
				UserName:  "user1",
				Timestamp: time.Now(),
			}
			entry2 := DetectedSpamInfo{
				GID:       "gr2",
				Text:      "spam2",
				UserID:    2,
				UserName:  "user2",
				Timestamp: time.Now(),
			}
			checks := []spamcheck.Response{{Name: "test", Spam: true}}

			err = spam1.Write(ctx, entry1, checks)
			s.Require().NoError(err)
			err = spam2.Write(ctx, entry2, checks)
			s.Require().NoError(err)

			entries1, err := spam1.Read(ctx)
			s.Require().NoError(err)
			s.Len(entries1, 1)
			s.Equal("spam1", entries1[0].Text)

			entries2, err := spam2.Read(ctx)
			s.Require().NoError(err)
			s.Len(entries2, 1)
			s.Equal("spam2", entries2[0].Text)
		})

		s.Run("approved users isolation", func() {
			au1, err := NewApprovedUsers(ctx, db1)
			s.Require().NoError(err)
			au2, err := NewApprovedUsers(ctx, db2)
			s.Require().NoError(err)

			user1 := approved.UserInfo{UserID: "1", UserName: "user1"}
			user2 := approved.UserInfo{UserID: "2", UserName: "user2"}

			err = au1.Write(ctx, user1)
			s.Require().NoError(err)
			err = au2.Write(ctx, user2)
			s.Require().NoError(err)

			users1, err := au1.Read(ctx)
			s.Require().NoError(err)
			s.Len(users1, 1)
			s.Equal("user1", users1[0].UserName)

			users2, err := au2.Read(ctx)
			s.Require().NoError(err)
			s.Len(users2, 1)
			s.Equal("user2", users2[0].UserName)
		})

		s.Run("locator isolation", func() {
			loc1, err := NewLocator(ctx, time.Hour, 10, db1)
			s.Require().NoError(err)
			loc2, err := NewLocator(ctx, time.Hour, 10, db2)
			s.Require().NoError(err)

			err = loc1.AddMessage(ctx, "msg1", 1, 1, "user1", 1)
			s.Require().NoError(err)
			err = loc2.AddMessage(ctx, "msg2", 2, 2, "user2", 2)
			s.Require().NoError(err)

			meta1, found := loc1.Message(ctx, "msg1")
			s.Require().True(found)
			s.Equal(int64(1), meta1.UserID)

			meta2, found := loc2.Message(ctx, "msg2")
			s.Require().True(found)
			s.Equal(int64(2), meta2.UserID)

			_, found = loc1.Message(ctx, "msg2")
			s.Require().False(found)
			_, found = loc2.Message(ctx, "msg1")
			s.Require().False(found)
		})
	})
}
