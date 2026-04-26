package webapi

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDMUsers_getDMUsersHandlerHTMX_ValidHTML(t *testing.T) {
	mockProvider := &mocks.DMUsersProviderMock{
		GetDMUsersFunc: func() []events.DMUser {
			return []events.DMUser{
				{UserID: 111, UserName: "bob", DisplayName: "Bob Smith", Timestamp: time.Now().Add(-5 * time.Minute)},
				{UserID: 222, UserName: "", DisplayName: "Alice", Timestamp: time.Now().Add(-2 * time.Hour)},
			}
		},
	}

	server := NewServer(Config{DMUsersProvider: mockProvider})
	req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	http.HandlerFunc(server.getDMUsersHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	assert.Contains(t, body, "<table")
	assert.Contains(t, body, "<thead>")
	assert.Contains(t, body, "<tbody>")
	assert.Contains(t, body, "</table>")

	assert.Contains(t, body, "111")
	assert.Contains(t, body, "Bob Smith")
	assert.Contains(t, body, "@bob")
	assert.Contains(t, body, "5m ago")
	assert.Contains(t, body, "222")
	assert.Contains(t, body, "Alice")
	assert.Contains(t, body, "2h ago")

	assert.Contains(t, body, "copyUserID( 111 , this)")
	assert.Contains(t, body, "copyUserID( 222 , this)")
	assert.Contains(t, body, "Copy ID")

	assert.Contains(t, body, `hx-get="/dm-users"`)
	assert.Contains(t, body, `hx-target="#dm-users-container"`)
	assert.Contains(t, body, "Refresh")
}

func TestDMUsers_settingsPageContainsDMUsersSection(t *testing.T) {
	detectorMock := &mocks.DetectorMock{
		GetLuaPluginNamesFunc: func() []string { return nil },
	}

	server := NewServer(Config{
		Version:  "1.0",
		Detector: detectorMock,
		Settings: Settings{SuperUsers: []string{"admin1"}},
	})

	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/settings", http.NoBody)
	require.NoError(t, err)

	handler := http.HandlerFunc(server.htmlSettingsHandler)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	assert.Contains(t, body, `id="dm-users-panel"`)
	assert.Contains(t, body, "Don't know your ID? Message the bot!")
	assert.Contains(t, body, `data-bs-toggle="collapse"`)
	assert.Contains(t, body, `data-bs-target="#dm-users-panel"`)

	assert.Contains(t, body, "How to find your Telegram User ID")
	assert.Contains(t, body, "Open a chat with the bot")
	assert.Contains(t, body, "Send any message")
	assert.Contains(t, body, "your ID will appear in the table")

	assert.Contains(t, body, `hx-get="/dm-users"`)
	assert.NotContains(t, body, `sse-connect="/dm-users/stream"`, "SSE should not be in settings page")

	assert.Contains(t, body, "function copyUserID(userId, btn)")
	assert.Contains(t, body, "navigator.clipboard")
}
