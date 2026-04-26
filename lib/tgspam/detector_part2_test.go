package tgspam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/mocks"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDetector_CheckDuplicatesTimeWindow(t *testing.T) {
	d := NewDetector(Config{
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 2,
			Window:    100 * time.Millisecond,
		},
	})

	spam, _ := d.Check(spamcheck.Request{Msg: "test", UserID: "123"})
	assert.False(t, spam)

	spam, cr := d.Check(spamcheck.Request{Msg: "test", UserID: "123"})
	assert.True(t, spam)
	dupResp := findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.True(t, dupResp.Spam)

	time.Sleep(150 * time.Millisecond)

	spam, _ = d.Check(spamcheck.Request{Msg: "test", UserID: "123"})
	assert.False(t, spam)
}

func TestDetector_CheckDuplicatesConcurrency(t *testing.T) {
	d := NewDetector(Config{
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 50,
			Window:    time.Minute,
		},
	})

	userID := "12345"
	message := "concurrent test message"
	concurrency := 10
	iterations := 10
	expectedCount := concurrency * iterations

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()
			for range iterations {
				d.Check(spamcheck.Request{
					UserID: userID,
					Msg:    message,
				})
			}
		}()
	}

	wg.Wait()

	spam, cr := d.Check(spamcheck.Request{
		UserID: userID,
		Msg:    message,
	})

	assert.True(t, spam, "should be detected as spam after %d messages", expectedCount+1)

	dupResp := findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.True(t, dupResp.Spam)

	// extract count from details
	var count int
	_, err := fmt.Sscanf(dupResp.Details, "message repeated %d times", &count)
	require.NoError(t, err)

	assert.Equal(t, expectedCount+1, count, "count should be accurate with no race conditions")
}

func TestDetector_CheckDuplicatesMemoryProtection(t *testing.T) {
	d := NewDetector(Config{
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 2,
			Window:    time.Hour,
		},
	})

	userID := "12345"

	for i := range 300 {
		message := fmt.Sprintf("unique message %d", i)
		d.Check(spamcheck.Request{
			UserID: userID,
			Msg:    message,
		})
	}

	spam, cr := d.Check(spamcheck.Request{
		UserID: userID,
		Msg:    "unique message 50",
	})
	assert.False(t, spam, "early message should have been evicted, not detected as duplicate")
	dupResp := findResponseByName(cr, "duplicate")
	assert.False(t, dupResp.Spam, "should not be spam since it was evicted")

	spam, cr = d.Check(spamcheck.Request{
		UserID: userID,
		Msg:    "unique message 250",
	})
	assert.True(t, spam, "recent message should be tracked and trigger spam detection")
	dupResp = findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.True(t, dupResp.Spam)
	assert.Contains(t, dupResp.Details, "message repeated 2 times")
}

func TestDetector_DuplicateDetectionForApprovedUsers(t *testing.T) {

	d := NewDetector(Config{
		FirstMessageOnly:   true,
		FirstMessagesCount: 1,
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 2,
			Window:    5 * time.Minute,
		},
	})

	userID := "8050302772"
	duplicateMsg := "Кто не против пообщатся"

	req1 := spamcheck.Request{
		Msg:    duplicateMsg,
		UserID: userID,
		Meta:   spamcheck.MetaData{MessageID: 658144},
	}
	spam1, results1 := d.Check(req1)

	assert.False(t, spam1, "first message should be ham")
	dupResp1 := findResponseByName(results1, "duplicate")
	require.NotNil(t, dupResp1, "duplicate check should have run")
	assert.False(t, dupResp1.Spam, "first message is not a duplicate")

	d.lock.RLock()
	userInfo := d.approvedUsers[userID]
	d.lock.RUnlock()
	assert.Equal(t, 1, userInfo.Count, "user should have count=1 after first message")

	req2 := spamcheck.Request{
		Msg:    duplicateMsg,
		UserID: userID,
		Meta:   spamcheck.MetaData{MessageID: 658145},
	}
	spam2, results2 := d.Check(req2)

	assert.True(t, spam2, "duplicate message should be detected as spam even for approved user")

	dupResp2 := findResponseByName(results2, "duplicate")
	require.NotNil(t, dupResp2, "duplicate check should have run for approved user")
	assert.True(t, dupResp2.Spam, "duplicate check should detect spam")
	assert.Contains(t, dupResp2.Details, "message repeated 2 times")

	assert.NotEmpty(t, dupResp2.ExtraDeleteIDs, "should have extra message IDs to delete")
	assert.Contains(t, dupResp2.ExtraDeleteIDs, 658144, "should include first message ID for deletion")
}

func TestDetector_DuplicateDetectionEdgeCases(t *testing.T) {
	t.Run("approved user with different messages - no false positive", func(t *testing.T) {

		d := NewDetector(Config{
			FirstMessageOnly:   true,
			FirstMessagesCount: 1,
			DuplicateDetection: struct {
				Threshold int
				Window    time.Duration
			}{
				Threshold: 2,
				Window:    5 * time.Minute,
			},
		})

		userID := "12345"

		spam1, _ := d.Check(spamcheck.Request{Msg: "first message", UserID: userID})
		assert.False(t, spam1)

		spam2, results2 := d.Check(spamcheck.Request{Msg: "second different message", UserID: userID})
		assert.False(t, spam2, "different message from approved user should be ham")

		preApproved := findResponseByName(results2, "pre-approved")
		assert.NotNil(t, preApproved, "should have pre-approved response")
	})

	t.Run("duplicate spam with ExtraDeleteIDs", func(t *testing.T) {

		d := NewDetector(Config{
			FirstMessageOnly:   true,
			FirstMessagesCount: 2,
			DuplicateDetection: struct {
				Threshold int
				Window    time.Duration
			}{
				Threshold: 3,
				Window:    5 * time.Minute,
			},
		})

		userID := "999"
		msg := "spam spam spam"

		d.Check(spamcheck.Request{Msg: msg, UserID: userID, Meta: spamcheck.MetaData{MessageID: 100}})
		d.Check(spamcheck.Request{Msg: msg, UserID: userID, Meta: spamcheck.MetaData{MessageID: 101}})
		spam3, results3 := d.Check(spamcheck.Request{Msg: msg, UserID: userID, Meta: spamcheck.MetaData{MessageID: 102}})

		assert.True(t, spam3, "third duplicate should be spam")
		dupResp := findResponseByName(results3, "duplicate")
		require.NotNil(t, dupResp)
		assert.True(t, dupResp.Spam)

		assert.Len(t, dupResp.ExtraDeleteIDs, 2, "should have 2 previous message IDs")
		assert.Contains(t, dupResp.ExtraDeleteIDs, 100)
		assert.Contains(t, dupResp.ExtraDeleteIDs, 101)
	})

	t.Run("pre-approval still works for non-duplicate content checks", func(t *testing.T) {

		d := NewDetector(Config{
			FirstMessageOnly:    true,
			FirstMessagesCount:  1,
			SimilarityThreshold: 0.8,
			MinMsgLen:           10,
			DuplicateDetection: struct {
				Threshold int
				Window    time.Duration
			}{
				Threshold: 2,
				Window:    5 * time.Minute,
			},
		})

		d.LoadSamples(bytes.NewBufferString(""), []io.Reader{bytes.NewBufferString("buy crypto now\nget rich quick")}, []io.Reader{bytes.NewBufferString("hello world")})

		userID := "777"

		spam1, _ := d.Check(spamcheck.Request{Msg: "first message here", UserID: userID})
		assert.False(t, spam1)

		spam2, results2 := d.Check(spamcheck.Request{Msg: "buy crypto now and get rich", UserID: userID})
		assert.False(t, spam2, "approved user should skip content checks")

		similarityResp := findResponseByName(results2, "similarity")
		assert.Nil(t, similarityResp, "similarity check should be skipped for approved users")

		dupResp := findResponseByName(results2, "duplicate")
		assert.NotNil(t, dupResp, "duplicate check should run for approved users")
	})
}

func TestSpam_CheckIsCasSpam(t *testing.T) {
	tests := []struct {
		name           string
		mockResp       string
		mockStatusCode int
		expected       bool
	}{
		{
			name:           "User is not a spammer",
			mockResp:       `{"ok": false, "description": "Not a spammer"}`,
			mockStatusCode: 200,
			expected:       false,
		},
		{
			name:           "User is not a spammer, message case",
			mockResp:       `{"ok": false, "description": "Not A spamMer."}`,
			mockStatusCode: 200,
			expected:       false,
		},
		{
			name:           "User is a spammer",
			mockResp:       `{"ok": true, "description": "Is a spammer"}`,
			mockStatusCode: 200,
			expected:       true,
		},
		{
			name:           "User is a spammer",
			mockResp:       `{"ok": true, "description": ""}`,
			mockStatusCode: 200,
			expected:       true,
		},
		{
			name:           "HTTP 503 service unavailable",
			mockResp:       `{"ok": false, "description": "not found"}`,
			mockStatusCode: 503,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockedHTTPClient := &mocks.HTTPClientMock{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewBufferString(tt.mockResp)),
					}, nil
				},
			}

			d := NewDetector(Config{
				CasAPI:           "http://localhost",
				HTTPClient:       mockedHTTPClient,
				MaxAllowedEmoji:  -1,
				FirstMessageOnly: true,
			})
			spam, cr := d.Check(spamcheck.Request{UserID: "123"})
			assert.Equal(t, tt.expected, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "cas", cr[0].Name)
			assert.Equal(t, tt.expected, cr[0].Spam)

			if tt.mockStatusCode >= 500 {
				assert.Contains(t, cr[0].Details, "failed to send request")
				assert.Greater(t, len(mockedHTTPClient.DoCalls()), 1, "should retry on 5xx errors")
			} else {
				respDetails := struct {
					OK          bool   `json:"ok"`
					Description string `json:"description"`
				}{}
				err := json.Unmarshal([]byte(tt.mockResp), &respDetails)
				require.NoError(t, err)
				expResp := strings.ToLower(respDetails.Description)
				if expResp == "" {
					expResp = "spam detected"
				}
				expResp = strings.TrimSuffix(expResp, ".")
				assert.Equal(t, expResp, cr[0].Details)
				assert.Len(t, mockedHTTPClient.DoCalls(), 1)
			}
		})
	}
}

func TestSpam_CheckIsCasSpamEmptyUserID(t *testing.T) {
	mockedHTTPClient := &mocks.HTTPClientMock{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			assert.Fail(t, "HTTP client should not be called with empty userID")
			return nil, nil
		},
	}

	d := NewDetector(Config{
		CasAPI:           "http://localhost",
		HTTPClient:       mockedHTTPClient,
		MaxAllowedEmoji:  -1,
		FirstMessageOnly: true,
	})
	spam, cr := d.Check(spamcheck.Request{UserID: "", Msg: "test message"})
	assert.False(t, spam)

	// find the CAS check in the results
	var casCheck *spamcheck.Response
	for _, check := range cr {
		if check.Name == "cas" {
			casCheck = &check
			break
		}
	}

	require.NotNil(t, casCheck, "CAS check should be included in results")
	assert.False(t, casCheck.Spam)
	assert.Equal(t, "check disabled", casCheck.Details)

	assert.Empty(t, mockedHTTPClient.DoCalls())
}
