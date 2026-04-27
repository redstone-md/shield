package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServer_routes(t *testing.T) {
	detectorMock := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "not spam"}}
		},
		ApprovedUsersFunc: func() []approved.UserInfo {
			return []approved.UserInfo{
				{UserID: "user1", UserName: "name1"},
				{UserID: "user2", UserName: "name2"}}
		},
		AddApprovedUserFunc: func(user approved.UserInfo) error {
			return nil
		},
		RemoveApprovedUserFunc: func(id string) error {
			return nil
		},
		GetLuaPluginNamesFunc: func() []string {
			return []string{"plugin1", "plugin2", "plugin3"}
		},
	}
	detectedSpamMock := &mocks.DetectedSpamMock{
		FindByUserIDFunc: func(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error) {
			if userID == 123 {
				return &storage.DetectedSpamInfo{
					ID:        123,
					GID:       "gid123",
					Text:      "spam example",
					UserID:    123,
					UserName:  "user",
					Checks:    []spamcheck.Response{{Spam: true, Name: "test", Details: "this was spam"}},
					Timestamp: time.Date(2025, 1, 25, 10, 0, 0, 0, time.UTC),
				}, nil
			}
			return nil, nil
		},
	}
	spamFilterMock := &mocks.SpamFilterMock{
		UpdateHamFunc:               func(msg string) error { return nil },
		UpdateSpamFunc:              func(msg string) error { return nil },
		RemoveDynamicSpamSampleFunc: func(sample string) error { return nil },
		RemoveDynamicHamSampleFunc:  func(sample string) error { return nil },
	}
	locatorMock := &mocks.LocatorMock{
		UserIDByNameFunc: func(ctx context.Context, userName string) int64 {
			if userName == "user1" {
				return 12345
			}
			return 0
		},
	}

	server := NewServer(Config{
		Detector:          detectorMock,
		SpamFilter:        spamFilterMock,
		Locator:           locatorMock,
		DetectedSpamStore: detectedSpamMock,
	})
	ts := httptest.NewServer(server.routes(routegroup.New(http.NewServeMux())))
	defer ts.Close()

	t.Run("check", func(t *testing.T) {
		detectorMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg":     "spam example",
			"user_id": "user123",
		})
		require.NoError(t, err)
		resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.CheckCalls(), 1)
		assert.Equal(t, "spam example", detectorMock.CheckCalls()[0].Req.Msg)
		assert.Equal(t, "user123", detectorMock.CheckCalls()[0].Req.UserID)
	})

	t.Run("check by id found", func(t *testing.T) {
		detectedSpamMock.ResetCalls()
		resp, err := http.Get(ts.URL + "/check/123")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectedSpamMock.FindByUserIDCalls(), 1)
		assert.Equal(t, int64(123), detectedSpamMock.FindByUserIDCalls()[0].UserID)
		assert.JSONEq(t, `{"status":"spam","info":{"user_name":"user","message":"spam example","timestamp":"2025-01-25T10:00:00Z","checks":[{"name":"test","spam":true,"details":"this was spam"}]}}`, strings.TrimSpace(string(body)))
	})

	t.Run("check by id not found", func(t *testing.T) {
		detectedSpamMock.ResetCalls()
		resp, err := http.Get(ts.URL + "/check/456")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectedSpamMock.FindByUserIDCalls(), 1)
		assert.Equal(t, int64(456), detectedSpamMock.FindByUserIDCalls()[0].UserID)
		assert.JSONEq(t, `{"status":"ham"}`, strings.TrimSpace(string(body)))
	})

	t.Run("update spam", func(t *testing.T) {
		detectorMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		resp, err := http.Post(ts.URL+"/update/spam", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, spamFilterMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("update ham", func(t *testing.T) {
		detectorMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		resp, err := http.Post(ts.URL+"/update/ham", "application/json", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, spamFilterMock.UpdateHamCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.UpdateHamCalls()[0].Msg)
	})

	t.Run("delete ham sample", func(t *testing.T) {
		detectorMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", ts.URL+"/delete/ham", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, spamFilterMock.RemoveDynamicHamSampleCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.RemoveDynamicHamSampleCalls()[0].Sample)
	})

	t.Run("delete spam sample", func(t *testing.T) {
		detectorMock.ResetCalls()
		reqBody, err := json.Marshal(map[string]string{
			"msg": "test message",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", ts.URL+"/delete/spam", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, spamFilterMock.RemoveDynamicSpamSampleCalls(), 1)
		assert.Equal(t, "test message", spamFilterMock.RemoveDynamicSpamSampleCalls()[0].Sample)
	})

	t.Run("add user", func(t *testing.T) {
		detectorMock.ResetCalls()
		locatorMock.ResetCalls()

		req, err := http.NewRequest("POST", ts.URL+"/users/add", bytes.NewBuffer([]byte(`{"user_id" : "123", "user_name":"user1"}`)))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.AddApprovedUserCalls(), 1)
		assert.Equal(t, "123", detectorMock.AddApprovedUserCalls()[0].User.UserID)
	})

	t.Run("add user without id", func(t *testing.T) {
		detectorMock.ResetCalls()
		locatorMock.ResetCalls()
		req, err := http.NewRequest("POST", ts.URL+"/users/add", bytes.NewBuffer([]byte(`{"user_name" : "user1"}`)))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.AddApprovedUserCalls(), 1)
		assert.Equal(t, "12345", detectorMock.AddApprovedUserCalls()[0].User.UserID)
		assert.Len(t, locatorMock.UserIDByNameCalls(), 1)
		assert.Equal(t, "user1", locatorMock.UserIDByNameCalls()[0].UserName)
	})

	t.Run("add user by name, not found", func(t *testing.T) {
		detectorMock.ResetCalls()
		locatorMock.ResetCalls()
		req, err := http.NewRequest("POST", ts.URL+"/users/add", bytes.NewBuffer([]byte(`{"user_name" : "user2"}`)))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Len(t, locatorMock.UserIDByNameCalls(), 1)
		assert.Equal(t, "user2", locatorMock.UserIDByNameCalls()[0].UserName)
	})

	t.Run("remove user by id", func(t *testing.T) {
		detectorMock.ResetCalls()
		locatorMock.ResetCalls()

		req, err := http.NewRequest("POST", ts.URL+"/users/delete", bytes.NewBuffer([]byte(`{"user_id" : "123"}`)))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, "123", detectorMock.RemoveApprovedUserCalls()[0].ID)
		assert.Empty(t, locatorMock.UserIDByNameCalls())
	})

	t.Run("remove user by name", func(t *testing.T) {
		detectorMock.ResetCalls()
		locatorMock.ResetCalls()
		req, err := http.NewRequest("POST", ts.URL+"/users/delete", bytes.NewBuffer([]byte(`{"user_name" : "user1"}`)))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, "12345", detectorMock.RemoveApprovedUserCalls()[0].ID)
		assert.Len(t, locatorMock.UserIDByNameCalls(), 1)
		assert.Equal(t, "user1", locatorMock.UserIDByNameCalls()[0].UserName)
	})

	t.Run("get approved users", func(t *testing.T) {
		detectorMock.ResetCalls()
		resp, err := http.Get(ts.URL + "/users")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Len(t, detectorMock.ApprovedUsersCalls(), 1)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"user_ids":[{"user_id":"user1","user_name":"name1","timestamp":"0001-01-01T00:00:00Z"},{"user_id":"user2","user_name":"name2","timestamp":"0001-01-01T00:00:00Z"}]}`, strings.TrimSpace(string(respBody)))
	})

	t.Run("get settings", func(t *testing.T) {
		server.Settings.MinMsgLen = 10
		resp, err := http.Get(ts.URL + "/settings")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))

		res := Settings{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, server.Settings, res)
	})
}

func TestServer_checkHandler(t *testing.T) {
	mockDetector := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			if req.UserID == "" {

				return false, []spamcheck.Response{
					{Details: "not spam"},
					{Name: "cas", Spam: false, Details: "check disabled"},
				}
			}

			if req.Msg == "spam example" {
				return true, []spamcheck.Response{{Spam: true, Name: "test", Details: "this was spam"}}
			}
			return false, []spamcheck.Response{{Details: "not spam"}}
		},
	}
	server := NewServer(Config{
		Detector: mockDetector,
		Version:  "1.0",
	})

	t.Run("spam", func(t *testing.T) {
		reqBody, err := json.Marshal(map[string]string{
			"msg":     "spam example",
			"user_id": "user123",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/check", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.checkMsgHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")

		var response struct {
			Spam   bool                 `json:"spam"`
			Checks []spamcheck.Response `json:"checks"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "error unmarshalling response")
		assert.True(t, response.Spam, "expected spam")
		assert.Equal(t, "test", response.Checks[0].Name, "unexpected check name")
		assert.Equal(t, "this was spam", response.Checks[0].Details, "unexpected check result")
	})

	t.Run("not spam", func(t *testing.T) {
		reqBody, err := json.Marshal(map[string]string{
			"msg":     "not spam example",
			"user_id": "user123",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/check", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.checkMsgHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")

		var response struct {
			Spam   bool                 `json:"spam"`
			Checks []spamcheck.Response `json:"checks"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "error unmarshalling response")
		assert.False(t, response.Spam, "expected not spam")
		assert.Equal(t, "not spam", response.Checks[0].Details, "unexpected check result")
	})

	t.Run("empty user ID", func(t *testing.T) {
		reqBody, err := json.Marshal(map[string]string{
			"msg":     "test message",
			"user_id": "",
		})
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/check", bytes.NewBuffer(reqBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.checkMsgHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")

		var response struct {
			Spam   bool                 `json:"spam"`
			Checks []spamcheck.Response `json:"checks"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "error unmarshalling response")

		// verify that the CAS check shows "check disabled"
		var casCheck *spamcheck.Response
		for _, check := range response.Checks {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}

		require.NotNil(t, casCheck, "CAS check should be included in results")
		assert.False(t, casCheck.Spam)
		assert.Equal(t, "check disabled", casCheck.Details)
	})

	t.Run("bad request", func(t *testing.T) {
		reqBody := []byte("bad request")
		req, err := http.NewRequest("POST", "/check", bytes.NewBuffer(reqBody))
		require.NoError(t, err)
		req.Body.Close()

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.checkMsgHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "handler returned wrong status code")
	})

}
