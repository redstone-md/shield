package bot

import (
	"context"
	"errors"
	"github.com/redstone-md/shield/app/bot/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/approved"
	"github.com/redstone-md/shield/lib/tgspam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestSpamFilter_UpdateSpam(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		updateErr   error
		expectError bool
	}{
		{
			name:        "successful update",
			message:     "spam message",
			expectError: false,
		},
		{
			name:        "update error",
			message:     "err",
			updateErr:   errors.New("update error"),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := &mocks.SampleUpdaterMock{
				UpdateSpamFunc: func(msg string) error { return tc.updateErr },
			}

			samplesStore := &mocks.SamplesStoreMock{}
			dictStore := &mocks.DictStoreMock{
				ReaderFunc: func(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, updater, nil, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			err := s.UpdateSpam(tc.message)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, updater.UpdateSpamCalls(), 1)
			assert.Equal(t, strings.ReplaceAll(tc.message, "\n", " "), updater.UpdateSpamCalls()[0].Msg)
		})
	}
}

func TestSpamFilter_UpdateHam(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		updateErr   error
		expectError bool
	}{
		{
			name:        "successful update",
			message:     "ham message",
			expectError: false,
		},
		{
			name:        "update error",
			message:     "err",
			updateErr:   errors.New("update error"),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := &mocks.SampleUpdaterMock{
				UpdateHamFunc: func(msg string) error { return tc.updateErr },
			}

			samplesStore := &mocks.SamplesStoreMock{}
			dictStore := &mocks.DictStoreMock{
				ReaderFunc: func(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, updater, nil, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			err := s.UpdateHam(tc.message)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, updater.UpdateHamCalls(), 1)
			assert.Equal(t, strings.ReplaceAll(tc.message, "\n", " "), updater.UpdateHamCalls()[0].Msg)
		})
	}
}

func TestSpamFilter_ApprovedUsers(t *testing.T) {
	tests := []struct {
		name         string
		userID       int64
		userName     string
		operation    string // "add" or "remove"
		operationErr error
		expectError  bool
	}{
		{
			name:        "add user success",
			userID:      123,
			userName:    "test_user",
			operation:   "add",
			expectError: false,
		},
		{
			name:         "add user operation error",
			userID:       -1,
			userName:     "test_user",
			operation:    "add",
			operationErr: errors.New("operation failed"),
			expectError:  true,
		},
		{
			name:        "remove user success",
			userID:      123,
			operation:   "remove",
			expectError: false,
		},
		{
			name:         "remove user operation error",
			userID:       -1,
			operation:    "remove",
			operationErr: errors.New("operation failed"),
			expectError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			approvedUsers := &mocks.ApprovedUsersMock{
				AddApprovedUserFunc: func(user approved.UserInfo) error {
					if tc.operationErr != nil {
						return tc.operationErr
					}
					assert.Equal(t, strconv.FormatInt(tc.userID, 10), user.UserID)
					assert.Equal(t, tc.userName, user.UserName)
					return nil
				},
				RemoveApprovedUserFunc: func(id string) error {
					if tc.operationErr != nil {
						return tc.operationErr
					}
					assert.Equal(t, strconv.FormatInt(tc.userID, 10), id)
					return nil
				},
				IsApprovedUserFunc: func(userID string) bool {
					return userID == strconv.FormatInt(tc.userID, 10)
				},
			}

			samplesStore := &mocks.SamplesStoreMock{
				StatsFunc: func(ctx context.Context) (*storage.SamplesStats, error) {
					return &storage.SamplesStats{PresetSpam: 1, PresetHam: 1}, nil
				},
				ReaderFunc: func(ctx context.Context, t storage.SampleType, o storage.SampleOrigin) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				},
			}

			dictStore := &mocks.DictStoreMock{
				ReaderFunc: func(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, nil, approvedUsers, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			var err error
			switch tc.operation {
			case "add":
				err = s.AddApprovedUser(tc.userID, tc.userName)
			case "remove":
				err = s.RemoveApprovedUser(tc.userID)
			}

			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			result := s.IsApprovedUser(tc.userID)
			if tc.operation == "add" && !tc.expectError {
				assert.True(t, result)
			}
		})
	}
}

func TestSpamFilter_ReloadSamples(t *testing.T) {
	tests := []struct {
		name         string
		statsResult  *storage.SamplesStats
		statsErr     error
		readerErr    error
		loadErr      error
		stopWordsErr error
		expectError  bool
	}{
		{
			name:        "successful reload",
			statsResult: &storage.SamplesStats{PresetSpam: 10, PresetHam: 5},
		},
		{
			name:        "no preset samples",
			statsResult: &storage.SamplesStats{},
			expectError: true,
		},
		{
			name:        "stats error",
			statsErr:    errors.New("stats error"),
			expectError: true,
		},
		{
			name:        "spam reader error",
			statsResult: &storage.SamplesStats{PresetSpam: 10, PresetHam: 5},
			readerErr:   errors.New("reader error"),
			expectError: true,
		},
		{
			name:        "load samples error",
			statsResult: &storage.SamplesStats{PresetSpam: 10, PresetHam: 5},
			loadErr:     errors.New("load error"),
			expectError: true,
		},
		{
			name:         "stop words error",
			statsResult:  &storage.SamplesStats{PresetSpam: 10, PresetHam: 5},
			stopWordsErr: errors.New("stop words error"),
			expectError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loader := &mocks.SampleLoaderMock{
				LoadSamplesFunc: func(exclReader io.Reader, spamReaders []io.Reader, hamReaders []io.Reader) (tgspam.LoadResult, error) {
					return tgspam.LoadResult{SpamSamples: 10, HamSamples: 5}, tc.loadErr
				},
				LoadStopWordsFunc: func(readers ...io.Reader) (tgspam.LoadResult, error) {
					return tgspam.LoadResult{StopWords: 3}, tc.stopWordsErr
				},
			}

			samplesStore := &mocks.SamplesStoreMock{
				StatsFunc: func(ctx context.Context) (*storage.SamplesStats, error) {
					return tc.statsResult, tc.statsErr
				},
				ReaderFunc: func(ctx context.Context, t storage.SampleType, o storage.SampleOrigin) (io.ReadCloser, error) {
					if tc.readerErr != nil {
						return nil, tc.readerErr
					}
					return io.NopCloser(strings.NewReader("test data")), nil
				},
			}

			dictStore := &mocks.DictStoreMock{
				ReaderFunc: func(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("test data")), nil
				},
			}

			s := NewSpamFilterWithRoles(nil, loader, nil, nil, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			err := s.ReloadSamples(context.Background())
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Len(t, loader.LoadSamplesCalls(), 1)
			assert.Len(t, loader.LoadStopWordsCalls(), 1)
			assert.Len(t, samplesStore.StatsCalls(), 1)
		})
	}
}

func TestSpamFilter_RemoveDynamicSample(t *testing.T) {
	tests := []struct {
		name        string
		sample      string
		deleteErr   error
		loadErr     error
		expectError bool
	}{
		{
			name:   "remove spam success",
			sample: "spam message",
		},
		{
			name:        "delete error",
			sample:      "spam message",
			deleteErr:   errors.New("delete error"),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := &mocks.SampleUpdaterMock{
				RemoveSpamFunc: func(msg string) error {
					assert.Equal(t, tc.sample, msg)
					return tc.deleteErr
				},
				RemoveHamFunc: func(msg string) error {
					assert.Equal(t, tc.sample, msg)
					return tc.deleteErr
				},
			}

			samplesStore := &mocks.SamplesStoreMock{}

			dictStore := &mocks.DictStoreMock{
				ReaderFunc: func(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, updater, nil, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			err := s.RemoveDynamicSpamSample(tc.sample)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, updater.RemoveSpamCalls(), 1)
			assert.Equal(t, tc.sample, updater.RemoveSpamCalls()[0].Msg)
		})
	}
}

func TestSpamFilter_IsApprovedUser(t *testing.T) {
	tests := []struct {
		name         string
		userID       int64
		expectedCall string
		want         bool
	}{
		{
			name:         "user is approved",
			userID:       123,
			expectedCall: "123",
			want:         true,
		},
		{
			name:         "user is not approved",
			userID:       456,
			expectedCall: "456",
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			approvedUsers := &mocks.ApprovedUsersMock{
				IsApprovedUserFunc: func(userID string) bool {
					assert.Equal(t, tc.expectedCall, userID)
					return tc.want
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, nil, approvedUsers, SpamConfig{})
			got := s.IsApprovedUser(tc.userID)
			assert.Equal(t, tc.want, got)
			assert.Len(t, approvedUsers.IsApprovedUserCalls(), 1)
		})
	}
}
