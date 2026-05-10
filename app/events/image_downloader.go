package events

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/image/webp"
)

type imageDownloader struct {
	api     TbAPI
	client  *http.Client
	maxSize int64
	extract func(context.Context, []byte, string, int64) ([]byte, string, error)
}

func newImageDownloader(api TbAPI) *imageDownloader {
	return &imageDownloader{
		api: api,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxSize: 10 * 1024 * 1024,
		extract: ffmpegExtractFirstFrameJPEG,
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

	detectedMime := http.DetectContentType(data)
	if isKnownNonImageMedia(detectedMime) {
		data, mime, err := d.extract(ctx, data, detectedMime, d.maxSize)
		if err != nil {
			return nil, "", err
		}
		return data, mime, nil
	}
	mime := detectedMime
	if !strings.HasPrefix(mime, "image/") {
		mime = resp.Header.Get("Content-Type")
	}
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	if detectedMime == "image/webp" {
		data, mime, err = convertWebPToJPEG(data)
		if err != nil {
			return nil, "", err
		}
	}
	if detectedMime == "image/jpeg" {
		data, mime, err = normalizeJPEG(data)
		if err != nil {
			return nil, "", err
		}
	}
	if int64(len(data)) > d.maxSize {
		return nil, "", fmt.Errorf("converted file exceeds max size: %d", d.maxSize)
	}

	return data, mime, nil
}

func isKnownNonImageMedia(mime string) bool {
	return strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
}

func ffmpegExtractFirstFrameJPEG(ctx context.Context, data []byte, mime string, maxSize int64) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-frames:v", "1", "-f", "image2", "-vcodec", "mjpeg", "pipe:1")
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("extract first frame from %s: %w: %s", mime, err, strings.TrimSpace(stderr.String()))
	}
	if int64(stdout.Len()) > maxSize {
		return nil, "", fmt.Errorf("extracted frame exceeds max size: %d", maxSize)
	}
	frame := stdout.Bytes()
	if http.DetectContentType(frame) != "image/jpeg" {
		return nil, "", fmt.Errorf("extract first frame from %s: unexpected output mime %s", mime, http.DetectContentType(frame))
	}
	return frame, "image/jpeg", nil
}

func convertWebPToJPEG(data []byte) ([]byte, string, error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("convert webp: decode: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", fmt.Errorf("convert webp: encode jpeg: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
}

func normalizeJPEG(data []byte) ([]byte, string, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("normalize jpeg: decode: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", fmt.Errorf("normalize jpeg: encode: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
}
