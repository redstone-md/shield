package tgspam

import (
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDuplicateDetector_EditedMessagesShouldNotTriggerSpam(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam, "original message should not be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam, "first edit should not be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam, "second edit should not be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam, "third edit should not be spam - edits of same message should not count as duplicates")

	resp = d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.False(t, resp.Spam, "second message not spam yet")

	resp = d.check(spamcheck.Request{
		Msg:    "hello world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1003},
	})
	assert.True(t, resp.Spam, "third DIFFERENT message should trigger spam (real duplicates)")
}

func TestDuplicateDetector_EditWithDifferentContent(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{
		Msg:    "world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam, "editing to different content should not be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.False(t, resp.Spam, "first 'hello' after edit")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1003},
	})
	assert.False(t, resp.Spam, "second 'hello' after edit")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1004},
	})
	assert.True(t, resp.Spam, "third 'hello' after edit should trigger spam")

	t.Logf("ExtraDeleteIDs: %v", resp.ExtraDeleteIDs)
	assert.NotContains(t, resp.ExtraDeleteIDs, 1001,
		"messageID 1001 should not be deleted - it now displays 'world', not 'hello'")

	assert.Equal(t, []int{1002, 1003}, resp.ExtraDeleteIDs,
		"should only delete actual 'hello' duplicates")
}

func TestDuplicateDetector_EditChangingContentDoesNotIncrementOldCount(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	resp := d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{
		Msg:    "world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1001},
	})
	assert.False(t, resp.Spam)

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.False(t, resp.Spam, "first 'hello' after edit")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1003},
	})
	assert.False(t, resp.Spam, "second 'hello' after edit should NOT trigger spam - only 2 visible 'hello' messages")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1004},
	})
	assert.True(t, resp.Spam, "third 'hello' after edit should trigger spam")
}

func TestDuplicateDetector_EditOldMessageWithinTimeWindow(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	for i := 1001; i <= 1005; i++ {
		resp := d.check(spamcheck.Request{
			Msg:    "hello",
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: i},
		})
		if i >= 1003 {
			assert.True(t, resp.Spam, "should be spam starting from 3rd message")
		}
	}

	resp := d.check(spamcheck.Request{
		Msg:    "world",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.False(t, resp.Spam, "single 'world' message should not be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1006},
	})
	assert.True(t, resp.Spam, "5 'hello' messages (1001,1003-1006) should be spam")

	resp = d.check(spamcheck.Request{
		Msg:    "hello",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.True(t, resp.Spam, "moving message back to 'hello' increases count from 5 to 6")
}

func TestDuplicateDetector_EmptyMessagesNotDuplicates(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	for i := range 5 {
		resp := d.check(spamcheck.Request{
			Msg:    "",
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: 1001 + i},
		})
		assert.False(t, resp.Spam, "empty message %d should not be flagged as spam", i)
		assert.Equal(t, "empty message skipped", resp.Details, "empty message %d", i)
	}

	for i, msg := range []string{"   ", "\t", "\n", " \t\n "} {
		resp := d.check(spamcheck.Request{
			Msg:    msg,
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: 2001 + i},
		})
		assert.False(t, resp.Spam, "whitespace-only message %q should not be flagged as spam", msg)
		assert.Equal(t, "empty message skipped", resp.Details, "whitespace-only message %q", msg)
	}
}

func TestDuplicateDetector_SameContentEditDoesNotRetriggerSpam(t *testing.T) {
	d := newDuplicateDetector(3, time.Hour)

	for i := 1001; i <= 1003; i++ {
		resp := d.check(spamcheck.Request{
			Msg:    "spam",
			UserID: "123",
			Meta:   spamcheck.MetaData{MessageID: i},
		})
		if i >= 1003 {
			assert.True(t, resp.Spam, "should trigger spam at 3rd message")
		}
	}

	resp := d.check(spamcheck.Request{
		Msg:    "spam",
		UserID: "123",
		Meta:   spamcheck.MetaData{MessageID: 1002},
	})
	assert.False(t, resp.Spam, "same-content edit should not re-trigger spam")
	assert.Empty(t, resp.ExtraDeleteIDs, "same-content edit should not return deletion IDs")
	assert.Equal(t, "message edit", resp.Details, "should indicate this is an edit")
}
