package events

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
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

func TestImageDownloader_download_prefersDetectedImageMime(t *testing.T) {
	webpData := []byte("RIFF\x1a\x00\x00\x00WEBPVP8 ")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(webpData)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file.jpg", nil
		},
	}

	d := newImageDownloader(mockAPI)
	data, mime, err := d.download(context.Background(), "fid")

	require.Error(t, err)
	assert.Nil(t, data)
	assert.Empty(t, mime)
	assert.Contains(t, err.Error(), "convert webp")
}

func TestImageDownloader_download_convertsWebPToJPEG(t *testing.T) {
	webpData, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		w.Write(webpData)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file.webp", nil
		},
	}

	d := newImageDownloader(mockAPI)
	data, mime, err := d.download(context.Background(), "fid")

	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
	assert.NotEqual(t, webpData, data)
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 1, 1), img.Bounds())
	assert.NotEqual(t, color.RGBA{}, img.At(0, 0))
}

func TestImageDownloader_download_normalizesJPEG(t *testing.T) {
	var original bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, jpeg.Encode(&original, img, &jpeg.Options{Quality: 75}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(original.Bytes())
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file.jpg", nil
		},
	}

	d := newImageDownloader(mockAPI)
	data, mime, err := d.download(context.Background(), "fid")

	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
	assert.NotEqual(t, original.Bytes(), data)
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 2, 2), decoded.Bounds())
}

func TestImageDownloader_downloadRejectsVideoBytesWithImageHeader(t *testing.T) {
	videoData := []byte("\x00\x00\x00\x20ftypisom\x00\x00\x02\x00isomiso2avc1mp41")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(videoData)
	}))
	defer srv.Close()

	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			return srv.URL + "/file.jpg", nil
		},
	}

	d := newImageDownloader(mockAPI)
	data, mime, err := d.download(context.Background(), "fid")

	require.Error(t, err)
	assert.Nil(t, data)
	assert.Empty(t, mime)
	assert.Contains(t, err.Error(), "unsupported media content")
	assert.Contains(t, err.Error(), "video/mp4")
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
