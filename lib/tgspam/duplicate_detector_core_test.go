package tgspam

import (
	"fmt"
	cache "github.com/go-pkgz/expirable-cache/v3"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestDuplicateDetector_Check(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		window    time.Duration
		messages  []spamcheck.Request
		expected  []bool // expected spam results for each message
	}{
		{
			name:      "disabled detector",
			threshold: 0,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "test", UserID: "123"},
				{Msg: "test", UserID: "123"},
			},
			expected: []bool{false, false},
		},
		{
			name:      "single message not spam",
			threshold: 2,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "hello", UserID: "123"},
			},
			expected: []bool{false},
		},
		{
			name:      "duplicate messages trigger spam",
			threshold: 3,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "spam", UserID: "123"},
				{Msg: "spam", UserID: "123"},
				{Msg: "spam", UserID: "123"},
				{Msg: "spam", UserID: "123"},
			},
			expected: []bool{false, false, true, true},
		},
		{
			name:      "different users don't trigger",
			threshold: 2,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "same", UserID: "123"},
				{Msg: "same", UserID: "456"},
				{Msg: "same", UserID: "123"},
			},
			expected: []bool{false, false, true},
		},
		{
			name:      "different messages don't trigger",
			threshold: 2,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "first", UserID: "123"},
				{Msg: "second", UserID: "123"},
				{Msg: "third", UserID: "123"},
			},
			expected: []bool{false, false, false},
		},
		{
			name:      "invalid user id",
			threshold: 2,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "test", UserID: "invalid"},
			},
			expected: []bool{false},
		},
		{
			name:      "empty user id",
			threshold: 2,
			window:    time.Hour,
			messages: []spamcheck.Request{
				{Msg: "test", UserID: ""},
			},
			expected: []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDuplicateDetector(tt.threshold, tt.window)

			for i, msg := range tt.messages {
				resp := d.check(msg)
				assert.Equal(t, tt.expected[i], resp.Spam,
					"message %d: %q from user %s", i, msg.Msg, msg.UserID)

				if resp.Spam {
					assert.Contains(t, resp.Details, "repeated")
				}

				if !resp.Spam && tt.threshold > 0 {

					if msg.UserID == "" {
						assert.Equal(t, "check disabled", resp.Details)
						continue
					}

					if _, err := strconv.ParseInt(msg.UserID, 10, 64); err != nil {
						assert.Equal(t, "invalid user id", resp.Details)
						continue
					}

					assert.Equal(t, "no duplicates found", resp.Details)
				}
			}
		})
	}
}

func TestDuplicateDetector_TimeWindow(t *testing.T) {
	d := newDuplicateDetector(2, 100*time.Millisecond)

	resp := d.check(spamcheck.Request{Msg: "test", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1001}})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{Msg: "test", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1002}})
	assert.True(t, resp.Spam)

	time.Sleep(150 * time.Millisecond)

	resp = d.check(spamcheck.Request{Msg: "test", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1003}})
	assert.False(t, resp.Spam)
}

func TestDuplicateDetector_ExtraDeleteIDs(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1001}})
	assert.False(t, resp.Spam)
	assert.Nil(t, resp.ExtraDeleteIDs)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1002}})
	assert.False(t, resp.Spam)
	assert.Nil(t, resp.ExtraDeleteIDs)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1003}})
	assert.True(t, resp.Spam)
	assert.Equal(t, []int{1001, 1002}, resp.ExtraDeleteIDs, "should return first two message IDs")

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1004}})
	assert.True(t, resp.Spam)
	assert.Equal(t, []int{1001, 1002, 1003}, resp.ExtraDeleteIDs, "should return all previous message IDs for deletion")
}

func TestDuplicateDetector_ExtraDeleteIDs_DifferentMessages(t *testing.T) {
	d := newDuplicateDetector(2, time.Hour)

	resp := d.check(spamcheck.Request{Msg: "hello", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1001}})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{Msg: "world", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1002}})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1003}})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1004}})
	assert.True(t, resp.Spam)
	assert.Equal(t, []int{1003}, resp.ExtraDeleteIDs, "should only include duplicate message ID")
}

func TestDuplicateDetector_AutomaticCleanup(t *testing.T) {
	d := newDuplicateDetector(3, 100*time.Millisecond)
	d.cleanupInterval = 50 * time.Millisecond

	for i := range 5 {
		d.check(spamcheck.Request{Msg: fmt.Sprintf("msg%d", i), UserID: fmt.Sprintf("%d", i)})
	}

	assert.Len(t, d.cache.Keys(), 5)

	time.Sleep(150 * time.Millisecond)

	d.check(spamcheck.Request{Msg: "trigger", UserID: "999"})

	keys := d.cache.Keys()
	assert.Len(t, keys, 1)
	assert.Equal(t, int64(999), keys[0])
}

func TestDuplicateDetector_CleanupInterval(t *testing.T) {
	d := newDuplicateDetector(3, 100*time.Millisecond)
	d.cleanupInterval = time.Hour

	for i := range 5 {
		d.check(spamcheck.Request{Msg: fmt.Sprintf("msg%d", i), UserID: fmt.Sprintf("%d", i)})
	}

	time.Sleep(150 * time.Millisecond)

	d.check(spamcheck.Request{Msg: "trigger", UserID: "999"})

	assert.Len(t, d.cache.Keys(), 6, "should have all 6 users, cleanup didn't run")
}

func TestDuplicateDetector_NilDetector(t *testing.T) {
	var d *duplicateDetector

	resp := d.check(spamcheck.Request{Msg: "test", UserID: "123"})
	assert.False(t, resp.Spam)
	assert.Equal(t, "check disabled", resp.Details)
}

func TestDuplicateDetector_ConcurrentAccess(t *testing.T) {
	d := newDuplicateDetector(5, time.Hour)

	done := make(chan bool, 10)
	for i := range 10 {
		go func(userID int) {
			for j := range 10 {
				d.check(spamcheck.Request{
					Msg:    fmt.Sprintf("msg%d", j%3),
					UserID: fmt.Sprintf("%d", userID),
				})
			}
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}

	keys := d.cache.Keys()
	assert.LessOrEqual(t, len(keys), 10)

	for _, userID := range keys {
		history, found := d.cache.Get(userID)
		require.True(t, found)
		require.NotNil(t, history)
		assert.Len(t, history.entries, 10)
		assert.Len(t, history.trackers, 3)
	}
}

func TestDuplicateDetector_DurationBetweenDuplicates(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1001}})
	assert.False(t, resp.Spam)

	time.Sleep(100 * time.Millisecond)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1002}})
	assert.False(t, resp.Spam)

	time.Sleep(200 * time.Millisecond)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1003}})
	assert.True(t, resp.Spam)

	assert.NotContains(t, resp.Details, "1h")
	assert.Contains(t, resp.Details, "message repeated 3 times in")

	assert.Contains(t, resp.Details, "message repeated 3 times in 0s")
}

func TestDuplicateDetector_InstantDuplicates(t *testing.T) {
	d := newDuplicateDetector(2, time.Hour)

	resp := d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1001}})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{Msg: "spam", UserID: "123", Meta: spamcheck.MetaData{MessageID: 1002}})
	assert.True(t, resp.Spam)

	assert.Contains(t, resp.Details, "message repeated 2 times in")

	assert.Regexp(t, `message repeated 2 times in (instantly|0s)`, resp.Details)
}

func TestDuplicateDetector_MessageIDsGrowthExceedsMax(t *testing.T) {
	d := newDuplicateDetector(1000, time.Hour)

	for i := range 300 {
		resp := d.check(spamcheck.Request{
			Msg:    "spam message",
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: 1000 + i},
		})
		assert.False(t, resp.Spam, "should not trigger spam with threshold 1000")
	}

	history, found := d.cache.Get(int64(123))
	assert.True(t, found)

	msgHash := d.hash("spam message")
	tracker := history.trackers[msgHash]

	t.Logf("maxEntriesPerUser: %d", d.maxEntriesPerUser)
	t.Logf("Number of entries: %d", len(history.entries))
	t.Logf("Number of message IDs stored: %d", len(tracker.messageIDs))

	assert.Len(t, history.entries, d.maxEntriesPerUser)

	assert.LessOrEqual(t, len(tracker.messageIDs), 100, "messageIDs should be capped to prevent unbounded growth")
	assert.NotEmpty(t, tracker.messageIDs, "should have some message IDs")
}

func TestDuplicateDetector_FirstSeenAfterTrimming(t *testing.T) {
	d := newDuplicateDetector(10, time.Hour)
	d.maxEntriesPerUser = 3

	for i := range 5 {
		resp := d.check(spamcheck.Request{
			Msg:    "same message",
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: 1000 + i},
		})
		assert.False(t, resp.Spam)
		time.Sleep(100 * time.Millisecond)
	}

	history, found := d.cache.Get(int64(123))
	assert.True(t, found)
	assert.Len(t, history.entries, 3, "should only keep 3 most recent entries")

	msgHash := d.hash("same message")
	tracker := history.trackers[msgHash]

	earliestEntry := history.entries[0]

	t.Logf("FirstSeen: %v", tracker.firstSeen)
	t.Logf("Earliest entry time: %v", earliestEntry.time)

	assert.Equal(t, earliestEntry.time, tracker.firstSeen, "firstSeen should be updated after trimming")
}

func TestDuplicateDetector_InvalidMessageIDs(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{
		Msg:    "test",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 0},
	})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{
		Msg:    "test",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: -1},
	})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{
		Msg:    "test",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.True(t, resp.Spam)

	t.Logf("ExtraDeleteIDs: %v", resp.ExtraDeleteIDs)
	for _, id := range resp.ExtraDeleteIDs {
		assert.Positive(t, id, "should not include invalid IDs (0 or negative)")
	}

}

func TestDuplicateDetector_LRUEviction(t *testing.T) {
	d := newDuplicateDetector(2, time.Hour)
	d.maxUsers = 3
	d.cache = cache.NewCache[int64, userHistory]().WithMaxKeys(3).WithTTL(time.Hour)

	for i := range 5 {
		d.check(spamcheck.Request{Msg: "test", UserID: fmt.Sprintf("%d", i)})
	}

	keys := d.cache.Keys()
	assert.LessOrEqual(t, len(keys), 3, "cache should respect max users limit")

	_, found := d.cache.Get(4)
	assert.True(t, found, "most recent user should be in cache")
}
