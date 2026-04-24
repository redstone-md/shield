package tgspam

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/tgspam/plugin"
)

// ApprovedUsers returns a list of approved users.
func (d *Detector) ApprovedUsers() (res []approved.UserInfo) {
	d.lock.RLock()
	defer d.lock.RUnlock()
	res = make([]approved.UserInfo, 0, len(d.approvedUsers))
	for _, info := range d.approvedUsers {
		res = append(res, info)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timestamp.After(res[j].Timestamp)
	})
	return res
}

// IsApprovedUser checks if a given user ID is approved.
// It uses memory cache for approved users and compares the count of messages sent by the user.
func (d *Detector) IsApprovedUser(userID string) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()

	ui, ok := d.approvedUsers[userID]
	if !ok {
		return false
	}
	return ui.Count > d.FirstMessagesCount
}

// AddApprovedUser adds user IDs to the list of approved users.
func (d *Detector) AddApprovedUser(user approved.UserInfo) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	ts := user.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	d.approvedUsers[user.UserID] = approved.UserInfo{
		UserID:    user.UserID,
		UserName:  user.UserName,
		Count:     d.FirstMessagesCount + 1, // +1 to skip first message check if count is 0
		Timestamp: ts,
	}

	if d.userStorage != nil {
		ctx, cancel := d.ctxWithStoreTimeout()
		defer cancel()
		if err := d.userStorage.Write(ctx, user); err != nil {
			return fmt.Errorf("failed to write approved user %+v to storage: %w", user, err)
		}
	}
	return nil
}

// RemoveApprovedUser removes approved user for given IDs
func (d *Detector) RemoveApprovedUser(id string) error {
	d.lock.Lock()
	delete(d.approvedUsers, id)
	d.lock.Unlock()

	if d.userStorage != nil {
		ctx, cancel := d.ctxWithStoreTimeout()
		defer cancel()
		if err := d.userStorage.Delete(ctx, id); err != nil {
			return fmt.Errorf("failed to delete approved user %s from storage: %w", id, err)
		}
	}
	return nil
}

// GetLuaPluginNames returns the list of available Lua plugin names.
func (d *Detector) GetLuaPluginNames() []string {
	d.lock.RLock()
	defer d.lock.RUnlock()

	if d.luaEngine == nil || !d.LuaPlugins.Enabled {
		return []string{}
	}

	allChecks := d.luaEngine.GetAllChecks()
	result := make([]string, 0, len(allChecks))

	for name := range allChecks {
		result = append(result, name)
	}

	// sort the result for consistent output
	sort.Strings(result)
	return result
}

// WithOpenAIChecker sets an openAIChecker for spam checking.
func (d *Detector) WithOpenAIChecker(client openAIClient, config OpenAIConfig) {
	d.openaiChecker = newOpenAIChecker(client, config)
}

// WithGeminiChecker sets a geminiChecker for spam checking.
func (d *Detector) WithGeminiChecker(client geminiClient, config GeminiConfig) {
	d.geminiChecker = newGeminiChecker(client, config)
}

// WithLuaEngine sets a Lua plugin engine and loads plugins
func (d *Detector) WithLuaEngine(engine LuaPluginEngine) error {
	d.luaEngine = engine

	if !d.LuaPlugins.Enabled || d.LuaPlugins.PluginsDir == "" {
		return nil
	}

	// load all plugins from the directory
	if err := d.luaEngine.LoadDirectory(d.LuaPlugins.PluginsDir); err != nil {
		return fmt.Errorf("failed to load Lua plugins: %w", err)
	}

	// register enabled plugins as Lua checks
	if len(d.LuaPlugins.EnabledPlugins) > 0 {
		for _, name := range d.LuaPlugins.EnabledPlugins {
			pluginCheck, err := d.luaEngine.GetCheck(name)
			if err != nil {
				return fmt.Errorf("failed to get Lua check %q: %w", name, err)
			}
			// add to luaChecks
			d.luaChecks = append(d.luaChecks, pluginCheck)
		}
	} else {
		// if no specific plugins are enabled, load all
		allChecks := d.luaEngine.GetAllChecks()
		for _, pluginCheck := range allChecks {
			// add to luaChecks
			d.luaChecks = append(d.luaChecks, pluginCheck)
		}
	}

	// set up a watcher for dynamic plugin reloading if enabled
	if d.LuaPlugins.DynamicReload {
		// we need to cast the luaEngine to a *plugin.Checker to access the watcher methods
		checker, ok := d.luaEngine.(*plugin.Checker)
		if !ok {
			log.Printf("[WARN] dynamic Lua plugin reloading enabled but engine doesn't support it")
			return nil
		}

		// create a watcher for the plugins directory
		watcher, err := plugin.NewWatcher(checker, d.LuaPlugins.PluginsDir)
		if err != nil {
			return fmt.Errorf("failed to create watcher for Lua plugins: %w", err)
		}

		// set the watcher on the checker
		checker.SetWatcher(watcher)

		// start the watcher
		if err := watcher.Start(); err != nil {
			return fmt.Errorf("failed to start watcher for Lua plugins: %w", err)
		}
	}

	return nil
}

// WithUserStorage sets a UserStorage for approved users and loads approved users from it.
func (d *Detector) WithUserStorage(storage UserStorage) (count int, err error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.approvedUsers = make(map[string]approved.UserInfo) // reset approved users
	d.userStorage = storage

	ctx, cancel := d.ctxWithStoreTimeout()
	defer cancel()

	users, err := d.userStorage.Read(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to read approved users from storage: %w", err)
	}
	for _, user := range users {
		user.Count = d.FirstMessagesCount + 1 // +1 to skip first message check if count is 0
		d.approvedUsers[user.UserID] = user
	}
	return len(users), nil
}

// WithMetaChecks sets a list of meta-checkers.
func (d *Detector) WithMetaChecks(mc ...MetaCheck) {
	d.metaChecks = append(d.metaChecks, mc...)
}

// ReplaceMetaChecks replaces the entire list of meta-checkers.
func (d *Detector) ReplaceMetaChecks(mc ...MetaCheck) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.metaChecks = mc
}

// UpdateConfig updates mutable detector configuration fields under write lock.
// Fields that cannot change after construction (CAS API, HTTP client, LLM checkers, Lua, history size)
// are left untouched. DuplicateDetection causes the duplicateDetector to be recreated.
func (d *Detector) UpdateConfig(cfg Config) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.SimilarityThreshold = cfg.SimilarityThreshold
	d.MinMsgLen = cfg.MinMsgLen
	d.MaxAllowedEmoji = cfg.MaxAllowedEmoji
	d.MinSpamProbability = cfg.MinSpamProbability
	d.MultiLangWords = cfg.MultiLangWords
	d.AbnormalSpacing = cfg.AbnormalSpacing
	if cfg.DuplicateDetection.Threshold != d.DuplicateDetection.Threshold || cfg.DuplicateDetection.Window != d.DuplicateDetection.Window {
		d.duplicateDetector = newDuplicateDetector(cfg.DuplicateDetection.Threshold, cfg.DuplicateDetection.Window)
		d.DuplicateDetection = cfg.DuplicateDetection
	}
}

// WithSpamUpdater sets a SampleUpdater for spam samples.
func (d *Detector) WithSpamUpdater(s SampleUpdater) { d.spamSamplesUpd = s }

// WithHamUpdater sets a SampleUpdater for ham samples.
func (d *Detector) WithHamUpdater(s SampleUpdater) { d.hamSamplesUpd = s }

func (d *Detector) ctxWithStoreTimeout() (context.Context, context.CancelFunc) {
	if d.StorageTimeout == 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), d.StorageTimeout)
}

const defaultLLMRequestTimeout = 30 * time.Second

func (d *Detector) ctxWithLLMTimeout() (context.Context, context.CancelFunc) {
	timeout := d.LLMRequestTimeout
	if timeout == 0 {
		timeout = defaultLLMRequestTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}
