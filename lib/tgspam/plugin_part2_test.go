package tgspam

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/plugin"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDetector_WithAPICheckLuaPlugin(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		assert.Equal(t, "POST", r.Method, "request method should be POST")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "Content-Type header should be set")
		assert.Equal(t, "application/json", r.Header.Get("Accept"), "Accept header should be set")

		// parse the request body
		var requestData map[string]any
		err := json.NewDecoder(r.Body).Decode(&requestData)
		assert.NoError(t, err, "should be able to decode request JSON")
		defer r.Body.Close()

		assert.Contains(t, requestData, "message", "request should contain message field")
		assert.Contains(t, requestData, "user_id", "request should contain user_id field")
		assert.Contains(t, requestData, "user_name", "request should contain user_name field")

		message, ok := requestData["message"].(string)
		assert.True(t, ok, "message should be a string")

		switch message {
		case "This message contains spam from API":

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"is_spam":    true,
				"confidence": 0.95,
				"reason":     "API spam pattern detected",
			})
		case "This causes an error response":

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "Internal server error",
			})
		case "This causes invalid JSON":

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{invalid json"))
		default:

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"is_spam":    false,
				"confidence": 0.1,
				"reason":     "No spam patterns detected",
			})
		}
	}))
	defer server.Close()

	modifiedAPICheck := plugin.NewChecker()

	tmpScript := filepath.Join("testdata", "api_check_test.lua")

	luaCode := fmt.Sprintf(`
-- api_check.lua (test version with mock server)
function check(req)
  -- Normalize the message
  local msg = to_lower(req.msg)
  
  -- Check for common spam patterns before making API requests
  if count_substring(msg, "crypto") > 0 or count_substring(msg, "investment") > 0 then
    return true, "local detection: spam keywords found"
  end
  
  -- For special test_mode handling
  if req.msg == "__test_mode__" then
    return false, "API check skipped in test mode"
  end
  
  -- Prepare data for the API request
  local request_data = {
    message = req.msg,
    user_id = req.user_id,
    user_name = req.user_name
  }
  
  -- Create headers
  local headers = {
    ["Content-Type"] = "application/json",
    ["Accept"] = "application/json"
  }
  
  -- Convert data to JSON
  local json_data, json_err = json_encode(request_data)
  if json_err then
    return false, "json encoding error: " .. json_err
  end
  
  -- Using test server URL
  local endpoint = "%s"
  
  -- Make a POST request to the mock API with a short timeout
  local response, status, err = http_request(endpoint, "POST", headers, json_data, 3)
  
  -- Handle request errors
  if err then
    return false, "API request failed: " .. err .. " (allowing message)"
  end
  
  -- Check for non-200 status codes
  if status ~= 200 then
    return false, "API returned status " .. status .. " (allowing message)"
  end
  
  -- Parse API response
  local result, parse_err = json_decode(response)
  if parse_err then
    return false, "API response parse error: " .. parse_err .. " (allowing message)"
  end
  
  -- Process API result
  if result.is_spam then
    return true, "API detection: " .. (result.reason or "unknown reason")
  end
  
  -- Message is not spam according to the API
  return false, "API verified: not spam"
end
`, server.URL)

	err := os.WriteFile(tmpScript, []byte(luaCode), 0o644)
	require.NoError(t, err, "should load modified api_check.lua")

	err = modifiedAPICheck.LoadScript(tmpScript)
	require.NoError(t, err, "should load the script")

	config := Config{}
	config.LuaPlugins.Enabled = true

	config.LuaPlugins.PluginsDir = ""

	allChecks := modifiedAPICheck.GetAllChecks()
	t.Logf("Available Lua checks: %v", allChecks)

	detector := NewDetector(config)
	err = detector.WithLuaEngine(modifiedAPICheck)
	require.NoError(t, err, "should initialize detector with Lua engine")

	for name, check := range allChecks {
		detector.luaChecks = append(detector.luaChecks, check)
		t.Logf("Added Lua check: %s", name)
	}

	t.Logf("Detector Lua checks: %d", len(detector.luaChecks))
	t.Logf("Active Lua plugin names: %v", detector.GetLuaPluginNames())

	findCheck := func(checks []spamcheck.Response, name string) *spamcheck.Response {
		for _, check := range checks {
			if check.Name == name {
				return &check
			}
		}
		return nil
	}

	t.Run("LocalSpamDetection", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "Great crypto opportunity!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.True(t, isSpam, "message with local spam keyword should be detected as spam")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.True(t, apiCheck.Spam, "api_check should detect this as spam")
		assert.Contains(t, apiCheck.Details, "local detection", "should indicate local detection")
	})

	t.Run("APISpamDetection", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "This message contains spam from API",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.True(t, isSpam, "message detected as spam by API should be marked as spam")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.True(t, apiCheck.Spam, "api_check should detect this as spam")
		assert.Contains(t, apiCheck.Details, "API detection", "should indicate API detection")
		assert.Contains(t, apiCheck.Details, "API spam pattern detected", "should include the reason from API")
	})

	t.Run("APICleanDetection", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "This is a perfectly normal message",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.False(t, isSpam, "clean message should not be marked as spam")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.False(t, apiCheck.Spam, "api_check should not mark this as spam")
		assert.Contains(t, apiCheck.Details, "API verified: not spam", "should indicate API verification")
	})

	t.Run("APIErrorHandling", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "This causes an error response",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.False(t, isSpam, "message causing API error should fail open (not spam)")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.False(t, apiCheck.Spam, "api_check should fail open on API error")
		assert.Contains(t, apiCheck.Details, "API returned status 500", "should indicate API error status")
	})

	t.Run("JSONParseErrorHandling", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "This causes invalid JSON",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.False(t, isSpam, "message causing JSON parse error should fail open (not spam)")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.False(t, apiCheck.Spam, "api_check should fail open on JSON parse error")
		assert.Contains(t, apiCheck.Details, "API response parse error", "should indicate JSON parse error")
	})

	t.Run("TestModeSkipsAPI", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "__test_mode__",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)

		assert.False(t, isSpam, "test mode message should not be marked as spam")
		apiCheck := findCheck(checks, "lua-api_check_test")
		require.NotNil(t, apiCheck, "api_check should be present")
		assert.False(t, apiCheck.Spam, "api_check should not mark test mode as spam")
		assert.Contains(t, apiCheck.Details, "skipped in test mode", "should indicate test mode")
	})

	os.Remove(tmpScript)
}

func TestDetector_WithLuaEngine_DynamicReload(t *testing.T) {

	tmpDir := t.TempDir()

	scriptPath := filepath.Join(tmpDir, "dynamic_reload.lua")
	err := os.WriteFile(scriptPath, []byte(`
function check(request)
	return false, "original plugin"
end
	`), 0o644)
	require.NoError(t, err)

	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = tmpDir
	config.LuaPlugins.DynamicReload = true

	detector := NewDetector(config)

	engine := plugin.NewChecker()
	defer engine.Close()

	err = detector.WithLuaEngine(engine)
	require.NoError(t, err)

	_, ok := detector.luaEngine.(*plugin.Checker)
	require.True(t, ok)

	names := detector.GetLuaPluginNames()
	assert.Contains(t, names, "dynamic_reload")

	req := spamcheck.Request{
		Msg:      "test message",
		UserID:   "user1",
		UserName: "testuser",
	}
	_, results := detector.Check(req)

	// find the plugin's result
	var luaResult *spamcheck.Response
	for _, res := range results {
		if res.Name == "lua-dynamic_reload" {
			luaResult = &res
			break
		}
	}

	require.NotNil(t, luaResult, "Lua plugin result should be present")
	assert.Equal(t, "original plugin", luaResult.Details)
	assert.False(t, luaResult.Spam)
}
