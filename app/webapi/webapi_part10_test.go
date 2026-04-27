package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type approvedUsersProviderSpy struct {
	list   func(ctx context.Context) ([]approved.UserInfo, error)
	add    func(ctx context.Context, user approved.UserInfo) error
	remove func(ctx context.Context, id string) error
}

func (s approvedUsersProviderSpy) List(ctx context.Context) ([]approved.UserInfo, error) {
	return s.list(ctx)
}
func (s approvedUsersProviderSpy) Add(ctx context.Context, user approved.UserInfo) error {
	return s.add(ctx, user)
}
func (s approvedUsersProviderSpy) Remove(ctx context.Context, id string) error {
	return s.remove(ctx, id)
}

type dictionaryProviderSpy struct {
	add        func(ctx context.Context, t storage.DictionaryType, data string) error
	delete     func(ctx context.Context, id int64) error
	read       func(ctx context.Context, t storage.DictionaryType) ([]string, error)
	readWithIDs func(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error)
	stats      func(ctx context.Context) (*storage.DictionaryStats, error)
}

func (s dictionaryProviderSpy) Add(ctx context.Context, t storage.DictionaryType, data string) error {
	return s.add(ctx, t, data)
}
func (s dictionaryProviderSpy) Delete(ctx context.Context, id int64) error       { return s.delete(ctx, id) }
func (s dictionaryProviderSpy) Read(ctx context.Context, t storage.DictionaryType) ([]string, error) {
	return s.read(ctx, t)
}
func (s dictionaryProviderSpy) ReadWithIDs(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
	return s.readWithIDs(ctx, t)
}
func (s dictionaryProviderSpy) Stats(ctx context.Context) (*storage.DictionaryStats, error) {
	return s.stats(ctx)
}

type detectedSpamProviderSpy struct {
	read              func(ctx context.Context) ([]storage.DetectedSpamInfo, error)
	findByUserID      func(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error)
	setAddedToSamples func(ctx context.Context, id int64) error
}

func (s detectedSpamProviderSpy) Read(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
	return s.read(ctx)
}
func (s detectedSpamProviderSpy) FindByUserID(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error) {
	return s.findByUserID(ctx, userID)
}
func (s detectedSpamProviderSpy) SetAddedToSamplesFlag(ctx context.Context, id int64) error {
	return s.setAddedToSamples(ctx, id)
}

func TestServer_routesThroughApprovedUsersProvider(t *testing.T) {
	addCalled := false
	removeCalled := false
	listCalled := false

	provider := approvedUsersProviderSpy{
		add: func(ctx context.Context, user approved.UserInfo) error {
			addCalled = true
			assert.Equal(t, "42", user.UserID)
			assert.Equal(t, "alice", user.UserName)
			return nil
		},
		remove: func(ctx context.Context, id string) error {
			removeCalled = true
			assert.Equal(t, "42", id)
			return nil
		},
		list: func(ctx context.Context) ([]approved.UserInfo, error) {
			listCalled = true
			return []approved.UserInfo{{UserID: "42", UserName: "alice"}}, nil
		},
	}

	detectorMock := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "ok"}}
		},
		GetLuaPluginNamesFunc: func() []string { return nil },
	}
	spamFilterMock := &mocks.SpamFilterMock{
		ReloadSamplesFunc: func() error { return nil },
	}

	server := NewServer(Config{
		Detector:              detectorMock,
		SpamFilter:            spamFilterMock,
		ApprovedUsersProvider: provider,
		Settings:              Settings{InstanceID: "gr1"},
	})
	ts := httptest.NewServer(server.routes(routegroup.New(http.NewServeMux())))
	defer ts.Close()

	t.Run("add user routes through provider", func(t *testing.T) {
		addCalled = false
		req, err := http.NewRequest("POST", ts.URL+"/users/add", bytes.NewBuffer([]byte(`{"user_id":"42","user_name":"alice"}`)))
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, addCalled)
		assert.Empty(t, detectorMock.AddApprovedUserCalls())
	})

	t.Run("delete user routes through provider", func(t *testing.T) {
		removeCalled = false
		req, err := http.NewRequest("POST", ts.URL+"/users/delete", bytes.NewBuffer([]byte(`{"user_id":"42"}`)))
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, removeCalled)
		assert.Empty(t, detectorMock.RemoveApprovedUserCalls())
	})

	t.Run("list users routes through provider", func(t *testing.T) {
		listCalled = false
		resp, err := http.Get(ts.URL + "/users")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, listCalled)
		assert.Empty(t, detectorMock.ApprovedUsersCalls())
	})
}

func TestServer_routesThroughDictionaryProvider(t *testing.T) {
	addCalled := false
	deleteCalled := false
	readCalled := false

	provider := dictionaryProviderSpy{
		add: func(ctx context.Context, dt storage.DictionaryType, data string) error {
			addCalled = true
			assert.Equal(t, storage.DictionaryTypeStopPhrase, dt)
			assert.Equal(t, "bad word", data)
			return nil
		},
		delete: func(ctx context.Context, id int64) error {
			deleteCalled = true
			assert.Equal(t, int64(7), id)
			return nil
		},
		read: func(ctx context.Context, dt storage.DictionaryType) ([]string, error) {
			readCalled = true
			return []string{"bad word"}, nil
		},
		readWithIDs: func(ctx context.Context, dt storage.DictionaryType) ([]storage.DictionaryEntry, error) {
			return nil, nil
		},
		stats: func(ctx context.Context) (*storage.DictionaryStats, error) {
			return nil, nil
		},
	}

	detectorMock := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "ok"}}
		},
		GetLuaPluginNamesFunc: func() []string { return nil },
	}
	dictStoreMock := &mocks.DictionaryMock{
		AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
			return nil
		},
	}
	spamFilterMock := &mocks.SpamFilterMock{
		ReloadSamplesFunc: func() error { return nil },
	}

	server := NewServer(Config{
		Detector:           detectorMock,
		SpamFilter:         spamFilterMock,
		DictionaryStore:    dictStoreMock,
		DictionaryProvider: provider,
		Settings:           Settings{InstanceID: "gr1"},
	})
	ts := httptest.NewServer(server.routes(routegroup.New(http.NewServeMux())))
	defer ts.Close()

	t.Run("add entry routes through provider", func(t *testing.T) {
		addCalled = false
		body, _ := json.Marshal(map[string]string{"type": "stop_phrase", "data": "bad word"})
		resp, err := http.Post(ts.URL+"/dictionary/add", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, addCalled)
		assert.Empty(t, dictStoreMock.AddCalls())
	})

	t.Run("delete entry routes through provider", func(t *testing.T) {
		deleteCalled = false
		body, _ := json.Marshal(map[string]int64{"id": 7})
		resp, err := http.Post(ts.URL+"/dictionary/delete", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, deleteCalled)
		assert.Empty(t, dictStoreMock.DeleteCalls())
	})

	t.Run("list entries routes through provider", func(t *testing.T) {
		readCalled = false
		resp, err := http.Get(ts.URL + "/dictionary")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, readCalled)
		assert.Empty(t, dictStoreMock.ReadCalls())
	})
}

func TestServer_routesThroughDetectedSpamProvider(t *testing.T) {
	findCalled := false
	readCalled := false

	provider := detectedSpamProviderSpy{
		findByUserID: func(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error) {
			findCalled = true
			assert.Equal(t, int64(123), userID)
			return &storage.DetectedSpamInfo{
				ID:       1,
				UserID:   123,
				UserName: "spammer",
				Text:     "spam text",
				Checks:   []spamcheck.Response{{Spam: true, Name: "test", Details: "detected"}},
			}, nil
		},
		read: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
			readCalled = true
			return []storage.DetectedSpamInfo{
				{ID: 1, UserID: 123, UserName: "spammer", Text: "spam text"},
			}, nil
		},
		setAddedToSamples: func(ctx context.Context, id int64) error { return nil },
	}

	detectorMock := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "ok"}}
		},
		GetLuaPluginNamesFunc: func() []string { return nil },
	}
	detectedSpamStoreMock := &mocks.DetectedSpamMock{
		FindByUserIDFunc: func(ctx context.Context, userID int64) (*storage.DetectedSpamInfo, error) {
			return nil, nil
		},
		ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
			return nil, nil
		},
	}
	spamFilterMock := &mocks.SpamFilterMock{
		ReloadSamplesFunc: func() error { return nil },
	}

	server := NewServer(Config{
		Detector:             detectorMock,
		SpamFilter:           spamFilterMock,
		DetectedSpamStore:    detectedSpamStoreMock,
		DetectedSpamProvider: provider,
		Settings:             Settings{InstanceID: "gr1"},
	})
	ts := httptest.NewServer(server.routes(routegroup.New(http.NewServeMux())))
	defer ts.Close()

	t.Run("check by id routes through provider", func(t *testing.T) {
		findCalled = false
		resp, err := http.Get(ts.URL + "/check/123")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, findCalled)
		assert.Empty(t, detectedSpamStoreMock.FindByUserIDCalls())

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Contains(t, string(body), "spammer")
	})

	t.Run("download detected spam routes through provider", func(t *testing.T) {
		readCalled = false
		req, err := http.NewRequest("GET", ts.URL+"/download/detected_spam", http.NoBody)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, readCalled)
		assert.Empty(t, detectedSpamStoreMock.ReadCalls())
	})
}
