package events

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type imageDownloader struct {
	api     TbAPI
	client  *http.Client
	maxSize int64
}

func newImageDownloader(api TbAPI) *imageDownloader {
	return &imageDownloader{
		api: api,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxSize: 10 * 1024 * 1024,
	}
}

func (d *imageDownloader) download(ctx context.Context, fileID string) ([]byte, string, error) {
	url, err := d.api.GetFileDirectURL(fileID)
	if err != nil {
		return nil, "", fmt.Errorf("get file url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download status: %d", resp.StatusCode)
	}

	if resp.ContentLength > d.maxSize {
		return nil, "", fmt.Errorf("file too large: %d > %d", resp.ContentLength, d.maxSize)
	}

	limited := io.LimitReader(resp.Body, d.maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	if int64(len(data)) > d.maxSize {
		return nil, "", fmt.Errorf("file exceeds max size: %d", d.maxSize)
	}

	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}

	return data, mime, nil
}
