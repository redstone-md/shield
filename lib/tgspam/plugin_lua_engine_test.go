package tgspam

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/mocks"
	"github.com/umputun/tg-spam/lib/tgspam/plugin"
	"sort"
	"testing"
)

func TestDetector_WithLuaEngine(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = "/path/to/plugins"
	config.LuaPlugins.EnabledPlugins = []string{"plugin1", "plugin2"}

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			assert.Equal(t, "/path/to/plugins", dir)
			return nil
		},
		GetCheckFunc: func(name string) (plugin.Check, error) {
			assert.Contains(t, []string{"plugin1", "plugin2"}, name)
			return func(req spamcheck.Request) spamcheck.Response {
				return spamcheck.Response{Name: "lua-" + name, Spam: true, Details: "test"}
			}, nil
		},
		CloseFunc: func() {

		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.NoError(t, err)

	assert.Len(t, mockLuaEngine.LoadDirectoryCalls(), 1)
	assert.Len(t, mockLuaEngine.GetCheckCalls(), 2)
	assert.Len(t, detector.luaChecks, 2)
	assert.Empty(t, detector.metaChecks)
}

func TestDetector_WithLuaEngine_Disabled(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = false

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			return nil
		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.NoError(t, err)

	assert.Empty(t, mockLuaEngine.LoadDirectoryCalls())
}

func TestDetector_WithLuaEngine_NoDirectory(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = ""

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			return nil
		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.NoError(t, err)

	assert.Empty(t, mockLuaEngine.LoadDirectoryCalls())
}

func TestDetector_WithLuaEngine_LoadError(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = "/path/to/plugins"

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			return errors.New("load error")
		},
		CloseFunc: func() {

		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load Lua plugins")
}

func TestDetector_WithLuaEngine_GetCheckError(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = "/path/to/plugins"
	config.LuaPlugins.EnabledPlugins = []string{"plugin1"}

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			return nil
		},
		GetCheckFunc: func(name string) (plugin.Check, error) {
			return nil, errors.New("get check error")
		},
		CloseFunc: func() {

		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get Lua check")
}

func TestDetector_WithLuaEngine_AllChecks(t *testing.T) {
	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = "/path/to/plugins"

	detector := NewDetector(config)

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		LoadDirectoryFunc: func(dir string) error {
			return nil
		},
		GetAllChecksFunc: func() map[string]plugin.Check {
			return map[string]plugin.Check{
				"plugin1": func(req spamcheck.Request) spamcheck.Response {
					return spamcheck.Response{Name: "lua-plugin1", Spam: true, Details: "test1"}
				},
				"plugin2": func(req spamcheck.Request) spamcheck.Response {
					return spamcheck.Response{Name: "lua-plugin2", Spam: false, Details: "test2"}
				},
			}
		},
		CloseFunc: func() {

		},
	}

	err := detector.WithLuaEngine(mockLuaEngine)
	require.NoError(t, err)

	assert.Len(t, mockLuaEngine.LoadDirectoryCalls(), 1)
	assert.Len(t, mockLuaEngine.GetAllChecksCalls(), 1)
	assert.Len(t, detector.luaChecks, 2)
	assert.Empty(t, detector.metaChecks)
}

func TestDetector_Reset_ClosesLuaEngine(t *testing.T) {
	detector := NewDetector(Config{})

	mockLuaEngine := &mocks.LuaPluginEngineMock{
		CloseFunc: func() {

		},
	}

	detector.luaEngine = mockLuaEngine

	detector.Reset()

	assert.Len(t, mockLuaEngine.CloseCalls(), 1)
	assert.Nil(t, detector.luaEngine)
	assert.Empty(t, detector.luaChecks)
}

func TestDetector_GetLuaPluginNames(t *testing.T) {
	t.Run("no Lua engine", func(t *testing.T) {
		detector := NewDetector(Config{})
		assert.Empty(t, detector.GetLuaPluginNames())
	})

	t.Run("Lua disabled", func(t *testing.T) {
		config := Config{}
		config.LuaPlugins.Enabled = false
		detector := NewDetector(config)

		mockLuaEngine := &mocks.LuaPluginEngineMock{
			GetAllChecksFunc: func() map[string]plugin.Check {
				return map[string]plugin.Check{
					"plugin1": nil,
					"plugin2": nil,
				}
			},
		}

		detector.luaEngine = mockLuaEngine
		assert.Empty(t, detector.GetLuaPluginNames())
		assert.Empty(t, mockLuaEngine.GetAllChecksCalls())
	})

	t.Run("with plugins", func(t *testing.T) {
		config := Config{}
		config.LuaPlugins.Enabled = true
		detector := NewDetector(config)

		mockLuaEngine := &mocks.LuaPluginEngineMock{
			GetAllChecksFunc: func() map[string]plugin.Check {
				return map[string]plugin.Check{
					"plugin1": func(req spamcheck.Request) spamcheck.Response {
						return spamcheck.Response{Name: "lua-plugin1", Spam: true, Details: "test"}
					},
					"plugin2": func(req spamcheck.Request) spamcheck.Response {
						return spamcheck.Response{Name: "lua-plugin2", Spam: false, Details: "test"}
					},
					"plugin3": func(req spamcheck.Request) spamcheck.Response {
						return spamcheck.Response{Name: "lua-plugin3", Spam: true, Details: "test"}
					},
				}
			},
		}

		detector.luaEngine = mockLuaEngine
		pluginNames := detector.GetLuaPluginNames()

		assert.Len(t, mockLuaEngine.GetAllChecksCalls(), 1)
		assert.Len(t, pluginNames, 3)

		sort.Strings(pluginNames)
		assert.Equal(t, []string{"plugin1", "plugin2", "plugin3"}, pluginNames)
	})
}

func TestDetector_WithRealLuaPlugins(t *testing.T) {

	config := Config{}
	config.LuaPlugins.Enabled = true
	config.LuaPlugins.PluginsDir = "./testdata"
	config.LuaPlugins.EnabledPlugins = []string{"domain_blacklist", "repeat_chars", "simple_test"}

	detector := NewDetector(config)
	engine := plugin.NewChecker()
	defer engine.Close()

	err := detector.WithLuaEngine(engine)
	require.NoError(t, err)

	assert.Len(t, detector.luaChecks, 3)

	findCheck := func(checks []spamcheck.Response, name string) *spamcheck.Response {
		for _, check := range checks {
			if check.Name == name {
				return &check
			}
		}
		return nil
	}

	t.Run("DomainBlacklist", func(t *testing.T) {

		req := spamcheck.Request{
			Msg:      "Check out https://suspicious.xyz for great deals!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)
		t.Logf("checks: %+v", checks)
		assert.True(t, isSpam, "message with suspicious domain should be detected as spam")
		domainCheck := findCheck(checks, "lua-domain_blacklist")
		assert.NotNil(t, domainCheck, "domain_blacklist check should be present")
		assert.True(t, domainCheck.Spam, "domain_blacklist should detect this as spam")
		assert.Contains(t, domainCheck.Details, "blacklisted TLD: .xyz", "should contain details about the blacklisted TLD")

		req = spamcheck.Request{
			Msg:      "Check out https://legitimate.com for great deals!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks = detector.Check(req)
		assert.False(t, isSpam, "message with legitimate domain shouldn't be detected as spam")
		domainCheck = findCheck(checks, "lua-domain_blacklist")
		if domainCheck != nil {
			assert.False(t, domainCheck.Spam, "legitimate domain shouldn't be detected as spam")
		}
	})

	t.Run("RepeatChars", func(t *testing.T) {
		req := spamcheck.Request{
			Msg:      "Hellooooooo everyone!!!!!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)
		assert.True(t, isSpam, "message with excessive repeating chars should be detected as spam")
		repeatCheck := findCheck(checks, "lua-repeat_chars")
		assert.NotNil(t, repeatCheck, "repeat_chars check should be present")
		assert.True(t, repeatCheck.Spam, "repeat_chars should detect this as spam")
		assert.Contains(t, repeatCheck.Details, "excessive repeated characters", "should contain details about excessive character repetition")

		req = spamcheck.Request{
			Msg:      "Hello everyone!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks = detector.Check(req)
		assert.False(t, isSpam, "normal message shouldn't be detected as spam")
		repeatCheck = findCheck(checks, "lua-repeat_chars")
		if repeatCheck != nil {
			assert.False(t, repeatCheck.Spam, "normal message shouldn't be detected as spam")
		}
	})

	t.Run("SimpleTest", func(t *testing.T) {
		req := spamcheck.Request{
			Msg:      "Great crypto investment opportunity!",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks := detector.Check(req)
		assert.True(t, isSpam, "message with spam keyword should be detected as spam")
		simpleCheck := findCheck(checks, "lua-simple_test")
		assert.NotNil(t, simpleCheck, "simple_test check should be present")
		assert.True(t, simpleCheck.Spam, "simple_test should detect this as spam")
		assert.Contains(t, simpleCheck.Details, "detected spam keyword: crypto", "should contain details about the detected keyword")

		req = spamcheck.Request{
			Msg:      "Just a normal message without keywords.",
			UserID:   "user1",
			UserName: "testuser",
		}
		isSpam, checks = detector.Check(req)
		assert.False(t, isSpam, "normal message shouldn't be detected as spam")
		simpleCheck = findCheck(checks, "lua-simple_test")
		if simpleCheck != nil {
			assert.False(t, simpleCheck.Spam, "normal message shouldn't be detected as spam")
		}
	})
}
