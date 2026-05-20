package tgspam

import (
	"bytes"
	"fmt"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/redstone-md/shield/lib/tgspam/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSpam_CheckIsCasSpamUserAgent(t *testing.T) {
	const customUserAgent = "MyCustomUserAgent/1.0"

	tests := []struct {
		name           string
		userAgent      string
		expectedHeader string
	}{
		{
			name:           "with custom user agent",
			userAgent:      customUserAgent,
			expectedHeader: customUserAgent,
		},
		{
			name:           "with default user agent",
			userAgent:      "",
			expectedHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockedHTTPClient := &mocks.HTTPClientMock{
				DoFunc: func(req *http.Request) (*http.Response, error) {

					if tt.expectedHeader != "" {
						assert.Equal(t, tt.expectedHeader, req.Header.Get("User-Agent"))
					} else {

						assert.NotEqual(t, customUserAgent, req.Header.Get("User-Agent"))
					}

					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(`{"ok": false, "description": "Not a spammer"}`)),
					}, nil
				},
			}

			d := NewDetector(Config{
				CasAPI:           "http://localhost",
				CasUserAgent:     tt.userAgent,
				HTTPClient:       mockedHTTPClient,
				MaxAllowedEmoji:  -1,
				FirstMessageOnly: true,
			})

			d.Check(spamcheck.Request{UserID: "123", Msg: "test message"})

			assert.Len(t, mockedHTTPClient.DoCalls(), 1)
		})
	}
}

func TestSpam_CheckIsCasSpamRetry(t *testing.T) {
	t.Run("retry on network failure then success", func(t *testing.T) {
		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				if callCount == 1 {
					return nil, fmt.Errorf("network error")
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok": false, "description": "not found"}`)),
				}, nil
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 2, callCount, "should retry once after network failure")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
	})

	t.Run("retry on 5xx error then success", func(t *testing.T) {
		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				if callCount == 1 {
					return &http.Response{
						StatusCode: 500,
						Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok": false, "description": "not found"}`)),
				}, nil
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 2, callCount, "should retry once after 5xx error")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				return nil, fmt.Errorf("network error")
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 3, callCount, "should attempt 3 times with Repeats=3")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
		assert.Contains(t, casCheck.Details, "failed to send request")
	})
}

func TestSpam_CheckIsCasSpamHTMLResponse(t *testing.T) {
	t.Run("retry on HTML response then success", func(t *testing.T) {

		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				if callCount == 1 {
					return &http.Response{
						StatusCode: 200,
						Header:     http.Header{"Content-Type": []string{"text/html"}},
						Body:       io.NopCloser(bytes.NewBufferString("<html><body>Error</body></html>")),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok": false, "description": "not found"}`)),
				}, nil
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 2, callCount, "should retry once after HTML response")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
	})

	t.Run("max retries on persistent HTML response", func(t *testing.T) {
		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(bytes.NewBufferString("<html><body>Error</body></html>")),
				}, nil
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 3, callCount, "should attempt 3 times with Repeats=3")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
		assert.Contains(t, casCheck.Details, "unexpected content type")
	})
}

func TestSpam_CheckIsCasSpamNon200Status(t *testing.T) {
	t.Run("retry on 4xx errors then success", func(t *testing.T) {
		callCount := 0
		mockedHTTPClient := &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				callCount++
				if callCount == 1 {
					return &http.Response{
						StatusCode: 429,
						Body:       io.NopCloser(bytes.NewBufferString("Rate limited")),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok": false, "description": "not found"}`)),
				}, nil
			},
		}

		d := NewDetector(Config{
			CasAPI:           "http://localhost",
			HTTPClient:       mockedHTTPClient,
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
		})
		spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, 2, callCount, "should retry once after 429 error")
		var casCheck *spamcheck.Response
		for _, check := range cr {
			if check.Name == "cas" {
				casCheck = &check
				break
			}
		}
		require.NotNil(t, casCheck)
		assert.False(t, casCheck.Spam)
	})

	t.Run("max retries on persistent 4xx errors", func(t *testing.T) {
		tests := []struct {
			name       string
			statusCode int
		}{
			{"400 Bad Request", 400},
			{"404 Not Found", 404},
			{"429 Too Many Requests", 429},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				callCount := 0
				mockedHTTPClient := &mocks.HTTPClientMock{
					DoFunc: func(req *http.Request) (*http.Response, error) {
						callCount++
						return &http.Response{
							StatusCode: tt.statusCode,
							Body:       io.NopCloser(bytes.NewBufferString("Error")),
						}, nil
					},
				}

				d := NewDetector(Config{
					CasAPI:           "http://localhost",
					HTTPClient:       mockedHTTPClient,
					MaxAllowedEmoji:  -1,
					FirstMessageOnly: true,
				})
				spam, cr := d.Check(spamcheck.Request{UserID: "123", Msg: "test"})
				assert.False(t, spam)
				assert.Equal(t, 3, callCount, "should attempt 3 times with Repeats=3")
				var casCheck *spamcheck.Response
				for _, check := range cr {
					if check.Name == "cas" {
						casCheck = &check
						break
					}
				}
				require.NotNil(t, casCheck)
				assert.False(t, casCheck.Spam)
				assert.Contains(t, casCheck.Details, "unexpected status")
			})
		}
	})
}

func TestDetector_CheckSimilarity(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1})
	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, nil)
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2}, lr)
	d.classifier.reset()
	assert.Len(t, d.tokenizedSpam, 2)
	t.Logf("%+v", d.tokenizedSpam)
	assert.Equal(t, map[string]int{"win": 1, "free": 1, "iphone": 1}, d.tokenizedSpam[0])
	assert.Equal(t, map[string]int{"lottery": 1, "prize": 1}, d.tokenizedSpam[1])

	tests := []struct {
		name      string
		message   string
		threshold float64
		expected  bool
	}{
		{"Not Spam", "Hello, how are you?", 0.5, false},
		{"Exact Match", "Win a free iPhone now!", 0.5, true},
		{"Similar Match", "You won a lottery prize!", 0.3, true},
		{"High Threshold", "You won a lottery prize!", 0.9, false},
		{"Partial Match", "win free", 0.9, false},
		{"Low Threshold", "win free", 0.8, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d.SimilarityThreshold = test.threshold
			spam, cr := d.Check(spamcheck.Request{Msg: test.message})
			assert.Equal(t, test.expected, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "similarity", cr[0].Name)
		})
	}
}

func TestDetector_CheckClassifier(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, MinSpamProbability: 60})
	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	hamsSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, []io.Reader{hamsSamples})
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2, HamSamples: 3}, lr)
	d.tokenizedSpam = nil
	assert.Equal(t, 5, d.classifier.nAllDocument)
	exp := map[string]map[spamClass]int{"win": {"spam": 1}, "free": {"spam": 1}, "iphone": {"spam": 1}, "lottery": {"spam": 1},
		"prize": {"spam": 1}, "hello": {"ham": 1}, "world": {"ham": 1}, "how": {"ham": 1}, "are": {"ham": 1}, "you": {"ham": 1},
		"have": {"ham": 1}, "good": {"ham": 1}, "day": {"ham": 1}}
	assert.Equal(t, exp, d.classifier.learningResults)

	tests := []struct {
		name     string
		message  string
		expected bool
		desc     string
	}{
		{"clean ham", "Hello, how are you?", false, "probability of ham: 92.83%"},
		{"clean spam", "Win a free iPhone now!", true, "probability of spam: 90.81%"},
		{"a little bit spam", "You won a free lottery iphone good day", true, "probability of spam: 66.23%"},
		{"spam below threshold", "You won a free lottery iphone have a good day", false, "probability of spam: 53.36%"},
		{"mostly ham", "win a good day", false, "probability of ham: 65.39%"},
		{"mostly spam", "free  blah another one user writes good things iPhone day", true, "probability of spam: 75.70%"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: test.message})
			assert.Equal(t, test.expected, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "classifier", cr[0].Name)
			assert.Equal(t, test.expected, cr[0].Spam)
			t.Logf("%+v", cr[0].Details)
			assert.Equal(t, test.desc, cr[0].Details)
		})
	}

	t.Run("without minSpamProbability", func(t *testing.T) {
		d.MinSpamProbability = 0
		spam, cr := d.Check(spamcheck.Request{Msg: "You won a free lottery iphone have a good day"})
		assert.True(t, spam)
		assert.Equal(t, "probability of spam: 53.36%", cr[0].Details)

	})
}

func TestDetector_CheckClassifierNoHam(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, MinSpamProbability: 60})
	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, nil)
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2, HamSamples: 0}, lr)
	d.tokenizedSpam = nil
	assert.Equal(t, 2, d.classifier.nAllDocument)
	assert.Equal(t, 2, d.classifier.nDocumentByClass["spam"])
	assert.Equal(t, 0, d.classifier.nDocumentByClass["ham"])
	exp := map[string]map[spamClass]int{"win": {"spam": 1}, "free": {"spam": 1}, "iphone": {"spam": 1},
		"lottery": {"spam": 1}, "prize": {"spam": 1}}
	assert.Equal(t, exp, d.classifier.learningResults)

	tests := []string{
		"Hello, how are you?",
		"Win a free iPhone now!",
		"You won a free lottery iphone good day",
		"You won a free lottery iphone have a good day",
		"win a good day",
		"free  blah another one user writes good things iPhone day",
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: test})
			assert.False(t, spam)
			require.Empty(t, cr)
		})
	}
}
