package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestServer_updateSampleHandler(t *testing.T) {
	spamFilterMock := &mocks.SpamFilterMock{
		UpdateSpamFunc: func(msg string) error {
			if msg == "error" {
				return assert.AnError
			}
			return nil
		},
		UpdateHamFunc: func(msg string) error {
			if msg == "error" {
				return assert.AnError
			}
			return nil
		},
	}

	server := NewServer(Config{SpamFilter: spamFilterMock})

	t.Run("successful update ham", func(t *testing.T) {
		spamFilterMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/update", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateSampleHandler(spamFilterMock.UpdateHam))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		var response struct {
			Updated bool   `json:"updated"`
			Msg     string `json:"msg"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Updated)
		assert.Equal(t, "test message", response.Msg)
		assert.Len(t, spamFilterMock.UpdateHamCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.UpdateHamCalls()[0].Msg)
	})

	t.Run("update ham with error", func(t *testing.T) {
		spamFilterMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "error",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/update", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateSampleHandler(spamFilterMock.UpdateHam))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code, "handler returned wrong status code")
		var response struct {
			Err     string `json:"error"`
			Details string `json:"details"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "can't update samples", response.Err)
		assert.Equal(t, "assert.AnError general error for testing", response.Details)
		assert.Len(t, spamFilterMock.UpdateHamCalls(), 1)
		assert.Equal(t, "error", spamFilterMock.UpdateHamCalls()[0].Msg)
	})

	t.Run("bad request", func(t *testing.T) {
		spamFilterMock.ResetCalls()
		reqBody := []byte("bad request")
		req, err := http.NewRequest("POST", "/update", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateSampleHandler(spamFilterMock.UpdateHam))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code, "handler returned wrong status code")
	})
}

func TestServer_deleteSampleHandler(t *testing.T) {
	spamFilterMock := &mocks.SpamFilterMock{
		RemoveDynamicHamSampleFunc: func(sample string) error { return nil },
		DynamicSamplesFunc: func() ([]string, []string, error) {
			return []string{"spam1", "spam2"}, []string{"ham1", "ham2"}, nil
		},
	}
	server := NewServer(Config{SpamFilter: spamFilterMock})

	t.Run("successful delete ham sample", func(t *testing.T) {
		spamFilterMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/delete/ham", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.deleteSampleHandler(spamFilterMock.RemoveDynamicHamSample))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		var response struct {
			Deleted bool   `json:"deleted"`
			Msg     string `json:"msg"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Deleted)
		assert.Equal(t, "test message", response.Msg)
		require.Len(t, spamFilterMock.RemoveDynamicHamSampleCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.RemoveDynamicHamSampleCalls()[0].Sample)
	})

	t.Run("delete ham sample from htmx", func(t *testing.T) {
		spamFilterMock.ResetCalls()
		req, err := http.NewRequest("POST", "/delete/ham", http.NoBody)
		require.NoError(t, err)
		req.Header.Add("HX-Request", "true")

		req.Form = url.Values{}
		req.Form.Set("msg", "test message")

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.deleteSampleHandler(spamFilterMock.RemoveDynamicHamSample))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		body := rr.Body.String()
		t.Log(body)
		assert.Contains(t, body, "Spam Samples (2)", "response should contain spam samples")
		assert.Contains(t, body, "Ham Samples (2)", "response should contain ham samples")
		require.Len(t, spamFilterMock.RemoveDynamicHamSampleCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.RemoveDynamicHamSampleCalls()[0].Sample)
	})

	t.Run("delete ham sample with error", func(t *testing.T) {
		spamFilterMock.RemoveDynamicHamSampleFunc = func(sample string) error { return assert.AnError }
		spamFilterMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/delete/ham", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.deleteSampleHandler(spamFilterMock.RemoveDynamicHamSample))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code, "handler returned wrong status code")
	})
}

func TestServer_updateApprovedUsersHandler(t *testing.T) {
	mockDetector := &mocks.DetectorMock{
		AddApprovedUserFunc: func(user approved.UserInfo) error {
			if user.UserID == "error" {
				return assert.AnError
			}
			return nil
		},
		ApprovedUsersFunc: func() []approved.UserInfo {
			return []approved.UserInfo{{UserID: "12345", UserName: "user1"}, {UserID: "67890", UserName: "user2"}}
		},
	}
	locatorMock := &mocks.LocatorMock{
		UserIDByNameFunc: func(ctx context.Context, userName string) int64 {
			if userName == "user1" {
				return 12345
			}
			return 0
		},
	}

	server := NewServer(Config{Detector: mockDetector, Locator: locatorMock})

	t.Run("successful update by name", func(t *testing.T) {
		mockDetector.ResetCalls()
		locatorMock.ResetCalls()

		req, err := http.NewRequest("POST", "/users/add", bytes.NewBuffer([]byte(`{"user_name" : "user1"}`)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateApprovedUsersHandler(server.Detector.AddApprovedUser))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		var response struct {
			Updated  bool   `json:"updated"`
			UserID   string `json:"user_id"`
			UserName string `json:"user_name"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Updated)
		assert.Equal(t, "12345", response.UserID)
		assert.Equal(t, "user1", response.UserName)
		assert.Len(t, mockDetector.AddApprovedUserCalls(), 1)
		assert.Equal(t, "12345", mockDetector.AddApprovedUserCalls()[0].User.UserID)
		assert.Len(t, locatorMock.UserIDByNameCalls(), 1)
		assert.Equal(t, "user1", locatorMock.UserIDByNameCalls()[0].UserName)
	})

	t.Run("successful update from htmx", func(t *testing.T) {
		mockDetector.ResetCalls()
		locatorMock.ResetCalls()

		req, err := http.NewRequest("POST", "/users/add", http.NoBody)
		require.NoError(t, err)
		req.Header.Add("HX-Request", "true")

		req.Form = url.Values{}
		req.Form.Set("user_id", "123")
		req.Form.Set("user_name", "user1")

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateApprovedUsersHandler(server.Detector.AddApprovedUser))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		body := rr.Body.String()
		t.Log(body)
		assert.Contains(t, body, "<h4>Approved Users (2)</h4>", "response should contain approved users header")
		assert.Contains(t, body, "user1")
		assert.Contains(t, body, "user2")

		assert.Len(t, mockDetector.AddApprovedUserCalls(), 1)
		assert.Equal(t, "123", mockDetector.AddApprovedUserCalls()[0].User.UserID)
		assert.Empty(t, locatorMock.UserIDByNameCalls())
	})

	t.Run("successful update by id", func(t *testing.T) {
		mockDetector.ResetCalls()
		locatorMock.ResetCalls()
		req, err := http.NewRequest("POST", "/users/add", bytes.NewBuffer([]byte(`{"user_id" : "123"}`)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateApprovedUsersHandler(server.Detector.AddApprovedUser))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")
		var response struct {
			Updated  bool   `json:"updated"`
			UserID   string `json:"user_id"`
			UserName string `json:"user_name"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Updated)
		assert.Equal(t, "123", response.UserID)
		assert.Empty(t, response.UserName)
		assert.Len(t, mockDetector.AddApprovedUserCalls(), 1)
		assert.Equal(t, "123", mockDetector.AddApprovedUserCalls()[0].User.UserID)
		assert.Empty(t, locatorMock.UserIDByNameCalls())
	})
	t.Run("bad request", func(t *testing.T) {
		mockDetector.ResetCalls()
		reqBody := []byte("bad request")
		req, err := http.NewRequest("POST", "/users/add", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.updateApprovedUsersHandler(server.Detector.AddApprovedUser))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code, "handler returned wrong status code")
	})
}

func TestServer_htmlDetectedSpamHandler(t *testing.T) {
	t.Run("successful rendering", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
				ts := time.Now()
				return []storage.DetectedSpamInfo{
					{
						Text:      "spam1 12345'",
						UserID:    12345,
						UserName:  "user1",
						Timestamp: ts,
					},
					{
						Text:      "spam2",
						UserID:    67890,
						UserName:  "user2",
						Timestamp: ts,
					},
				}, nil
			},
		}
		server := NewServer(Config{DetectedSpam: ds})

		req, err := http.NewRequest("GET", "/detected_spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()

		assert.Contains(t, body, "Detected Spam")
		assert.Contains(t, body, `href="/download/detected_spam"`)
		assert.Contains(t, body, "btn-custom-blue")

		assert.Contains(t, body, "spam1 12345")
		assert.Contains(t, body, "user1")
		assert.Contains(t, body, "12345")
		assert.Contains(t, body, "spam2")
		assert.Contains(t, body, "user2")
		assert.Contains(t, body, "67890")
	})

	t.Run("read failure", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
				return nil, errors.New("test error")
			},
		}
		server := NewServer(Config{DetectedSpam: ds})

		req, err := http.NewRequest("GET", "/detected_spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestServer_htmlAddDetectedSpamHandler(t *testing.T) {
	t.Run("successful addition", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			SetAddedToSamplesFlagFunc: func(ctx context.Context, id int64) error {
				return nil
			},
		}
		sf := &mocks.SpamFilterMock{
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}
		server := NewServer(Config{DetectedSpam: ds, SpamFilter: sf})
		req, err := http.NewRequest("POST", "/detected_spam/add?id=123&msg=blah", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlAddDetectedSpamHandler)

		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Len(t, ds.SetAddedToSamplesFlagCalls(), 1)
		assert.Equal(t, int64(123), ds.SetAddedToSamplesFlagCalls()[0].ID)
		assert.Len(t, sf.UpdateSpamCalls(), 1)
		assert.Equal(t, "blah", sf.UpdateSpamCalls()[0].Msg)
	})

	t.Run("bad request - missing ID", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{}
		sf := &mocks.SpamFilterMock{}
		server := NewServer(Config{DetectedSpam: ds, SpamFilter: sf})

		req, err := http.NewRequest("POST", "/detected_spam/add?msg=blah", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlAddDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Contains(t, rr.Header().Get("HX-Retarget"), "#error-message")
		assert.Contains(t, rr.Body.String(), "bad request")
	})

	t.Run("bad request - missing message", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{}
		sf := &mocks.SpamFilterMock{}
		server := NewServer(Config{DetectedSpam: ds, SpamFilter: sf})

		req, err := http.NewRequest("POST", "/detected_spam/add?id=123", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlAddDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Contains(t, rr.Header().Get("HX-Retarget"), "#error-message")
		assert.Contains(t, rr.Body.String(), "bad request")
	})

	t.Run("update spam error", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{}
		sf := &mocks.SpamFilterMock{
			UpdateSpamFunc: func(msg string) error {
				return errors.New("update error")
			},
		}
		server := NewServer(Config{DetectedSpam: ds, SpamFilter: sf})

		req, err := http.NewRequest("POST", "/detected_spam/add?id=123&msg=blah", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlAddDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Contains(t, rr.Header().Get("HX-Retarget"), "#error-message")
		assert.Contains(t, rr.Body.String(), "can't update spam samples")
	})

	t.Run("set flag error", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			SetAddedToSamplesFlagFunc: func(ctx context.Context, id int64) error {
				return errors.New("flag update error")
			},
		}
		sf := &mocks.SpamFilterMock{
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}
		server := NewServer(Config{DetectedSpam: ds, SpamFilter: sf})

		req, err := http.NewRequest("POST", "/detected_spam/add?id=123&msg=blah", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.htmlAddDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Contains(t, rr.Header().Get("HX-Retarget"), "#error-message")
		assert.Contains(t, rr.Body.String(), "can't update detected spam")
	})
}

func TestServer_GenerateRandomPassword(t *testing.T) {
	res1, err := GenerateRandomPassword(32)
	require.NoError(t, err)
	t.Log(res1)
	assert.Len(t, res1, 32)

	res2, err := GenerateRandomPassword(32)
	require.NoError(t, err)
	t.Log(res2)
	assert.Len(t, res2, 32)

	assert.NotEqual(t, res1, res2)
}
