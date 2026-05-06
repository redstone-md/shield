package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-pkgz/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServer_Run(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(Config{ListenAddr: ":9876", Version: "dev", Detector: &mocks.DetectorMock{},
		SpamFilter: &mocks.SpamFilterMock{}, AuthPasswd: "test"})
	done := make(chan struct{})
	go func() {
		err := srv.Run(ctx)
		assert.NoError(t, err)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:9876/ping")
	require.NoError(t, err)
	t.Log(resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))

	assert.Contains(t, resp.Header.Get("App-Name"), "tg-spam")
	assert.Contains(t, resp.Header.Get("App-Version"), "dev")

	cancel()
	<-done
}

func TestServer_RequestMetadataMiddlewarePropagatesToDetectedSpam(t *testing.T) {
	var capturedCtx context.Context

	server := NewServer(Config{
		Settings: Settings{TenantID: "inst"},
		DetectedSpamStore: &mocks.DetectedSpamMock{
			FindByUserIDFunc: func(ctx context.Context, tenantID string, userID int64) (*storage.DetectedSpamInfo, error) {
				capturedCtx = ctx
				return &storage.DetectedSpamInfo{UserID: userID, UserName: "user"}, nil
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/check/42", nil)
	req.SetPathValue("user_id", "42")
	req.Header.Set("X-Request-ID", "req-123")
	rr := httptest.NewRecorder()

	handler := server.requestMetadataMiddleware(http.HandlerFunc(server.checkIDHandler))
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "req-123", rr.Header().Get(requestHeaderCorrelationID))
	assert.NotEmpty(t, rr.Header().Get(requestHeaderEventID))

	meta, ok := observability.MetadataFromContext(capturedCtx)
	require.True(t, ok)
	assert.Equal(t, rr.Header().Get(requestHeaderEventID), meta.EventID)
	assert.Equal(t, "req-123", meta.CorrelationID)
}

func TestServer_CheckMsgHandlerLogsRequestMetadata(t *testing.T) {
	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	}()

	server := NewServer(Config{Settings: Settings{TenantID: "inst"}})
	req := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader("bad request"))
	req.Header.Set("X-Request-ID", "req-456")
	rr := httptest.NewRecorder()

	handler := server.requestMetadataMiddleware(http.HandlerFunc(server.checkMsgHandler))
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, buf.String(), "evt=web-inst-")
	assert.Contains(t, buf.String(), "corr=req-456")
	assert.Equal(t, "req-456", rr.Header().Get(requestHeaderCorrelationID))
}

func TestServer_RunAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mockDetector := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "not spam"}}
		},
	}
	mockSpamFilter := &mocks.SpamFilterMock{}

	hashedPassword, err := rest.GenerateBcryptHash("test")
	require.NoError(t, err)
	t.Logf("hashed password: %s", hashedPassword)

	tests := []struct {
		name      string
		srv       *Server
		port      string
		authType  string
		password  string
		useHashed bool
	}{
		{
			name: "plain password auth",
			srv: NewServer(Config{
				ListenAddr: ":9877",
				Version:    "dev",
				Detector:   mockDetector,
				SpamFilter: mockSpamFilter,
				AuthPasswd: "test",
			}),
			port:     "9877",
			authType: "plain",
			password: "test",
		},
		{
			name: "bcrypt hash auth",
			srv: NewServer(Config{
				ListenAddr: ":9878",
				Version:    "dev",
				Detector:   mockDetector,
				SpamFilter: mockSpamFilter,
				AuthHash:   hashedPassword,
			}),
			port:      "9878",
			authType:  "hash",
			password:  "test",
			useHashed: true,
		},
	}

	doneChannels := make([]chan struct{}, 0, len(tests))
	for _, tc := range tests {
		done := make(chan struct{})
		doneChannels = append(doneChannels, done)
		t.Run(tc.authType, func(t *testing.T) {
			go func() {
				err := tc.srv.Run(ctx)
				assert.NoError(t, err)
				close(done)
			}()

			require.Eventually(t, func() bool {
				resp, err := http.Get(fmt.Sprintf("http://localhost:%s/ping", tc.port))
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, time.Second*2, time.Millisecond*50, "server did not start")

			t.Run("ping", func(t *testing.T) {
				resp, err := http.Get(fmt.Sprintf("http://localhost:%s/ping", tc.port))
				require.NoError(t, err)
				t.Log(resp)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			})

			t.Run("check unauthorized, no basic auth", func(t *testing.T) {
				resp, err := http.Get(fmt.Sprintf("http://localhost:%s/check", tc.port))
				require.NoError(t, err)
				t.Log(resp)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				if tc.useHashed {
					assert.Equal(t, `Basic realm="restricted", charset="UTF-8"`, resp.Header.Get("WWW-Authenticate"))
				}
			})

			t.Run("check authorized", func(t *testing.T) {
				reqBody, err := json.Marshal(map[string]string{
					"msg":     "spam example",
					"user_id": "user123",
				})
				require.NoError(t, err)
				req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%s/check", tc.port), bytes.NewBuffer(reqBody))
				require.NoError(t, err)
				req.SetBasicAuth("tg-spam", tc.password)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				t.Log(resp)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			})

			t.Run("wrong basic auth", func(t *testing.T) {
				reqBody, err := json.Marshal(map[string]string{
					"msg":     "spam example",
					"user_id": "user123",
				})
				require.NoError(t, err)
				req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%s/check", tc.port), bytes.NewBuffer(reqBody))
				require.NoError(t, err)
				req.SetBasicAuth("tg-spam", "bad")
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				t.Log(resp)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				if tc.useHashed {
					assert.Equal(t, `Basic realm="restricted", charset="UTF-8"`, resp.Header.Get("WWW-Authenticate"))
				}
			})
		})
	}
	cancel()
	for _, done := range doneChannels {
		<-done
	}
}
