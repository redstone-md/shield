package events

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/events/mocks"
)

func TestImageDownloader_download_success(t *testing.T) {
	imgData := []byte("fake-image-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(imgData)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file.png", nil
		},
	}

	d := newImageDownloader(mockAPI)
	data, mime, err := d.download(context.Background(), "test-file-id")

	require.NoError(t, err)
	assert.Equal(t, imgData, data)
	assert.Equal(t, "image/png", mime)
	assert.Len(t, mockAPI.GetFileDirectURLCalls(), 1)
	assert.Equal(t, "test-file-id", mockAPI.GetFileDirectURLCalls()[0].FileID)
}

func TestImageDownloader_download_defaultMime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "")
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file", nil
		},
	}

	d := newImageDownloader(mockAPI)
	_, mime, err := d.download(context.Background(), "fid")

	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
}

func TestImageDownloader_download_getURLFails(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return "", errors.New("test failure")
		},
	}

	d := newImageDownloader(mockAPI)
	_, _, err := d.download(context.Background(), "fid")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get file url")
}

func TestImageDownloader_download_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/missing", nil
		},
	}

	d := newImageDownloader(mockAPI)
	_, _, err := d.download(context.Background(), "fid")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "download status: 404")
}

func TestImageDownloader_download_tooLarge(t *testing.T) {
	bigData := make([]byte, 11*1024*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "11534336")
		w.Write(bigData)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/big.png", nil
		},
	}

	d := newImageDownloader(mockAPI)
	_, _, err := d.download(context.Background(), "fid")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

func TestImageDownloader_download_contextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file", nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := newImageDownloader(mockAPI)
	_, _, err := d.download(ctx, "fid")

	require.Error(t, err)
}
