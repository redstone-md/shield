package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"io"
	"os"
	"path"
	"testing"
	"time"
)

func TestMakeSpamLogger(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "log")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	logger, err := makeSpamLogger(context.Background(), "gr1", file, db)
	require.NoError(t, err)

	msg := &bot.Message{
		From: bot.User{
			ID:          123,
			DisplayName: "Test User",
			Username:    "testuser",
		},
		Text: "Test message\nblah blah  \n\n\n",
	}

	response := &bot.Response{
		Text: "spam detected",
		CheckResults: []spamcheck.Response{
			{Name: "Check1", Spam: true, Details: "Details 1"},
			{Name: "Check2", Spam: false, Details: "Details 2"},
		},
	}

	logger.Save(msg, response)
	file.Close()

	file, err = os.Open(file.Name())
	require.NoError(t, err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		t.Log(line)

		var logEntry map[string]any
		err = json.Unmarshal([]byte(line), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "Test User", logEntry["display_name"])
		assert.Equal(t, "testuser", logEntry["user_name"])
		assert.InDelta(t, float64(123), logEntry["user_id"], 0.001)
		assert.Equal(t, "Test message blah blah", logEntry["text"])
	}
	require.NoError(t, scanner.Err())

	savedMsgs := []storage.DetectedSpamInfo{}
	err = db.Select(&savedMsgs, "SELECT text, user_id, user_name, timestamp, checks FROM detected_spam")
	require.NoError(t, err)
	require.Len(t, savedMsgs, 1)
	assert.Equal(t, "Test message blah blah", savedMsgs[0].Text)
	assert.Equal(t, "testuser", savedMsgs[0].UserName)
	assert.Equal(t, int64(123), savedMsgs[0].UserID)
	assert.JSONEq(t, `[{"name":"Check1","spam":true,"details":"Details 1"},{"name":"Check2","spam":false,"details":"Details 2"}]`,
		savedMsgs[0].ChecksJSON)

}

func TestMakeSpamLogger_SaveAudit(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "log")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	logger, err := makeSpamLogger(context.Background(), "gr1", file, db)
	require.NoError(t, err)

	enriched, ok := logger.(interface {
		SaveAudit(context.Context, events.AuditRecord) error
	})
	require.True(t, ok)

	record := events.AuditRecord{
		Event:          moderation.IncomingEvent{IdempotencyKey: "telegram:update:55:chat:66:message:77:edited:0"},
		Message:        &bot.Message{Text: "Test message\nblah", From: bot.User{ID: 123, DisplayName: "Test User", Username: "testuser"}},
		Decision:       moderation.PolicyDecision{Score: 2},
		RuleSetVersion: 7,
		Response:       bot.Response{},
	}
	record.Response.CheckResults = []spamcheck.Response{
		{Name: "duplicates", Spam: true, Details: "dup"},
		{Name: "openai", Spam: true, Details: "llm"},
	}

	err = enriched.SaveAudit(context.Background(), record)
	require.NoError(t, err)

	savedMsgs := []storage.DetectedSpamInfo{}
	err = db.Select(&savedMsgs, "SELECT text, user_id, user_name, checks, signal_source, score, matched_rules, rule_set_version, idempotency_key FROM detected_spam")
	require.NoError(t, err)
	require.Len(t, savedMsgs, 1)
	assert.Equal(t, "duplicates", savedMsgs[0].SignalSource)
	assert.Equal(t, 2.0, savedMsgs[0].Score)
	assert.Equal(t, 7, savedMsgs[0].RuleSetVersion)
	assert.Equal(t, "telegram:update:55:chat:66:message:77:edited:0", savedMsgs[0].IdempotencyKey)
	assert.JSONEq(t, `["duplicates","openai"]`, savedMsgs[0].MatchedRulesJSON)
}

func TestMakeSpamLogWriter(t *testing.T) {
	setupLog(true, "super-secret-token")
	t.Run("happy path", func(t *testing.T) {
		file, err := os.CreateTemp(os.TempDir(), "log")
		require.NoError(t, err)
		defer os.Remove(file.Name())

		var opts options
		opts.Logger.Enabled = true
		opts.Logger.FileName = file.Name()
		opts.Logger.MaxSize = "1M"
		opts.Logger.MaxBackups = 1

		writer, err := makeSpamLogWriter(opts)
		require.NoError(t, err)

		_, err = writer.Write([]byte("Test log entry\n"))
		require.NoError(t, err)
		err = writer.Close()
		require.NoError(t, err)

		file, err = os.Open(file.Name())
		require.NoError(t, err)

		content, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, "Test log entry\n", string(content))
	})

	t.Run("failed on wrong size", func(t *testing.T) {
		var opts options
		opts.Logger.Enabled = true
		opts.Logger.FileName = "/tmp"
		opts.Logger.MaxSize = "1f"
		opts.Logger.MaxBackups = 1
		writer, err := makeSpamLogWriter(opts)
		require.Error(t, err)
		t.Log(err)
		assert.Nil(t, writer)
	})

	t.Run("disabled", func(t *testing.T) {
		var opts options
		opts.Logger.Enabled = false
		opts.Logger.FileName = "/tmp"
		opts.Logger.MaxSize = "10M"
		opts.Logger.MaxBackups = 1
		writer, err := makeSpamLogWriter(opts)
		require.NoError(t, err)
		assert.IsType(t, nopWriteCloser{}, writer)
	})
}

func Test_makeDetector(t *testing.T) {
	t.Run("no options", func(t *testing.T) {
		var opts options
		res := makeDetector(opts)
		assert.NotNil(t, res)
	})

	t.Run("with first msgs count", func(t *testing.T) {
		var opts options
		opts.OpenAI.Token = "123"
		opts.Files.SamplesDataPath = "/tmp"
		opts.Files.DynamicDataPath = "/tmp"
		opts.FirstMessagesCount = 10
		res := makeDetector(opts)
		assert.NotNil(t, res)
		assert.Equal(t, 10, res.FirstMessagesCount)
		assert.True(t, res.FirstMessageOnly)
	})

	t.Run("with first msgs count and paranoid", func(t *testing.T) {
		var opts options
		opts.OpenAI.Token = "123"
		opts.Files.SamplesDataPath = "/tmp"
		opts.Files.DynamicDataPath = "/tmp"
		opts.FirstMessagesCount = 10
		opts.ParanoidMode = true
		res := makeDetector(opts)
		assert.NotNil(t, res)
		assert.Equal(t, 0, res.FirstMessagesCount)
		assert.False(t, res.FirstMessageOnly)
	})

	t.Run("uses rule set values", func(t *testing.T) {
		var opts options
		opts.OpenAI.Token = "123"
		opts.Gemini.Token = "456"

		ruleSet := rules.RuleSet{
			Meta: rules.MetaRules{
				LinksLimit:      2,
				MentionsLimit:   1,
				Keyboard:        true,
				Forwarded:       true,
				UsernameSymbols: "$",
			},
			Duplicates: rules.DuplicateRules{
				Threshold: 3,
				Window:    2 * time.Minute,
			},
			AbnormalSpacing: rules.AbnormalSpacingRules{
				Enabled:                 true,
				ShortWordLen:            4,
				ShortWordRatioThreshold: 0.5,
				SpaceRatioThreshold:     0.2,
				MinWords:                7,
			},
			OpenAI: rules.LLMRules{
				Enabled:            true,
				Veto:               true,
				Model:              "gpt-test",
				HistorySize:        4,
				CheckShortMessages: true,
			},
			Gemini: rules.LLMRules{
				Enabled:            true,
				Veto:               true,
				Model:              "gemini-test",
				HistorySize:        5,
				CheckShortMessages: true,
			},
		}

		res := makeDetectorWithRuleSet(opts, ruleSet)
		require.NotNil(t, res)
		assert.Equal(t, 3, res.DuplicateDetection.Threshold)
		assert.Equal(t, 2*time.Minute, res.DuplicateDetection.Window)
		assert.True(t, res.AbnormalSpacing.Enabled)
		assert.Equal(t, 4, res.AbnormalSpacing.ShortWordLen)
		assert.Equal(t, 0.5, res.AbnormalSpacing.ShortWordRatioThreshold)
		assert.Equal(t, 0.2, res.AbnormalSpacing.SpaceRatioThreshold)
		assert.Equal(t, 7, res.AbnormalSpacing.MinWordsCount)
		assert.True(t, res.OpenAIVeto)
		assert.Equal(t, 4, res.OpenAIHistorySize)
		assert.True(t, res.GeminiVeto)
		assert.Equal(t, 5, res.GeminiHistorySize)
	})
}

func TestReadPromptOverride(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		prompt, err := readPromptOverride(t.TempDir())
		require.NoError(t, err)
		assert.Empty(t, prompt)
	})

	t.Run("trims markdown prompt", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(path.Join(dir, promptOverrideFile), []byte("\ncustom prompt\n"), 0o600))

		prompt, err := readPromptOverride(dir)
		require.NoError(t, err)
		assert.Equal(t, "custom prompt", prompt)
	})
}

func TestResolveSlowPathPrompt(t *testing.T) {
	assert.Equal(t, "provider prompt", resolveSlowPathPrompt("provider prompt", "file prompt"))
	assert.Equal(t, "file prompt", resolveSlowPathPrompt(" ", "file prompt"))
	assert.Empty(t, resolveSlowPathPrompt("", ""))
}

func Test_makeSpamBot(t *testing.T) {
	ctx := t.Context()

	t.Run("no options", func(t *testing.T) {
		var opts options
		_, err := makeSpamBot(ctx, opts, rules.RuleSet{}, nil, nil)
		assert.Error(t, err)
	})

	t.Run("with valid options", func(t *testing.T) {
		var opts options
		tmpDir := t.TempDir()

		opts.Files.SamplesDataPath = tmpDir
		opts.Files.DynamicDataPath = tmpDir
		opts.InstanceID = "gr1"
		detector := makeDetector(opts)
		db, err := engine.NewSqlite(path.Join(tmpDir, "tg-spam.db"), "gr1")
		require.NoError(t, err)
		defer db.Close()

		samplesStore, err := storage.NewSamples(ctx, db)
		require.NoError(t, err)
		err = samplesStore.Add(ctx, storage.SampleTypeSpam, storage.SampleOriginPreset, "spam1")
		require.NoError(t, err)
		err = samplesStore.Add(ctx, storage.SampleTypeHam, storage.SampleOriginPreset, "ham1")
		require.NoError(t, err)

		res, err := makeSpamBot(ctx, opts, bootstrapRuleSet(opts), db, detector)
		require.NoError(t, err)
		assert.NotNil(t, res)
	})
}

func TestBootstrapRuleSet(t *testing.T) {
	var opts options
	opts.InstanceID = "gr1"
	opts.Meta.LinksLimit = 3
	opts.Meta.Keyboard = true
	opts.Duplicates.Threshold = 2
	opts.Duplicates.Window = time.Minute
	opts.AbnormalSpacing.Enabled = true
	opts.AbnormalSpacing.SpaceRatioThreshold = 0.4
	opts.Moderation.FirstStrike = 30 * time.Minute
	opts.Moderation.SecondStrike = 6 * time.Hour
	opts.Report.Enabled = true
	opts.Report.Threshold = 2
	opts.OpenAI.Model = "gpt-4o-mini"
	opts.OpenAI.CheckShortMessages = true
	opts.Gemini.Model = "gemma"
	opts.SoftBan = true
	opts.Dry = true

	got := bootstrapRuleSet(opts)

	assert.Equal(t, "gr1", got.WorkspaceID)
	assert.Equal(t, "bootstrap", got.Source)
	assert.Equal(t, 3, got.Meta.LinksLimit)
	assert.True(t, got.Meta.Keyboard)
	assert.Equal(t, 2, got.Duplicates.Threshold)
	assert.Equal(t, time.Minute, got.Duplicates.Window)
	assert.True(t, got.AbnormalSpacing.Enabled)
	assert.Equal(t, 0.4, got.AbnormalSpacing.SpaceRatioThreshold)
	assert.Equal(t, 30*time.Minute, got.Moderation.FirstStrike)
	assert.True(t, got.Moderation.SoftBan)
	assert.True(t, got.Moderation.DryRun)
	assert.True(t, got.Reports.Enabled)
	assert.Equal(t, 2, got.Reports.Threshold)
	assert.Equal(t, "gpt-4o-mini", got.OpenAI.Model)
	assert.True(t, got.OpenAI.CheckShortMessages)
	assert.Equal(t, "gemma", got.Gemini.Model)
}

func TestAssembleRuntimeBootstrapsRuleSet(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	var opts options
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db"))
	opts.Files.SamplesDataPath = tmpDir
	opts.Files.DynamicDataPath = tmpDir
	opts.Moderation.FirstStrike = 30 * time.Minute
	opts.Moderation.SecondStrike = 6 * time.Hour

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	require.NotNil(t, assembly.RuleSets)
	require.NotNil(t, assembly.IncomingEventsStore)
	require.NotNil(t, assembly.ModerationActionsStore)

	active, err := assembly.RuleSets.Active(ctx, "gr1")
	require.NoError(t, err)
	assert.Equal(t, "gr1", active.WorkspaceID)
	assert.Equal(t, "bootstrap", active.Source)
	assert.Equal(t, 30*time.Minute, active.Moderation.FirstStrike)
}

func TestAssembleRuntimeUsesActiveRuleSet(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	var opts options
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db"))
	opts.Files.SamplesDataPath = tmpDir
	opts.Files.DynamicDataPath = tmpDir
	opts.Moderation.FirstStrike = 30 * time.Minute
	opts.Report.Threshold = 2
	opts.Dry = false
	opts.SoftBan = false
	t.Setenv("DRY", "false")
	t.Setenv("SOFT_BAN", "false")

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	first, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	first.close()

	db, err := engine.New(ctx, fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db")), opts.InstanceID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		"INSERT INTO rule_set_versions (workspace_id, gid, tenant_id, version, source, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"gr1", opts.InstanceID, opts.InstanceID, 2, "test", `{"workspace_id":"gr1","version":2,"source":"test","meta":{"links_limit":5,"mentions_limit":4},"duplicates":{"threshold":4,"window":60000000000},"abnormal_spacing":{"enabled":true,"space_ratio_threshold":0.25,"short_word_ratio_threshold":0.6,"short_word_len":5,"min_words":8},"moderation":{"first_strike":900000000000,"second_strike":21600000000000,"soft_ban":true,"dry_run":true},"reports":{"enabled":true,"threshold":5,"auto_ban_threshold":6,"rate_limit":2,"rate_period":120000000000},"openai":{"enabled":false,"veto":true,"model":"gpt-active","history_size":3,"check_short_messages":true},"gemini":{"enabled":false,"veto":false,"model":"gemini-active","history_size":7,"check_short_messages":false}}`,
		time.Now().UTC(),
	)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		"UPDATE rule_sets SET active_version = ?, updated_at = ? WHERE workspace_id = ? AND tenant_id = ?",
		2, time.Now().UTC(), "gr1", opts.InstanceID,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	assert.Equal(t, 2, assembly.ActiveRuleSet.Version)
	require.NotNil(t, assembly.Detector)
	assert.Equal(t, 4, assembly.Detector.DuplicateDetection.Threshold)
	assert.True(t, assembly.Detector.AbnormalSpacing.Enabled)
	assert.Equal(t, 3, assembly.Detector.OpenAIHistorySize)

	tbAPI := &tbapi.BotAPI{Self: tbapi.User{UserName: "bot"}}
	listener := assembly.makeTelegramListener(opts, tbAPI)
	assert.Equal(t, 15*time.Minute, listener.ModerationConfig.FirstStrike)
	assert.False(t, listener.SoftBanMode)
	assert.False(t, listener.Dry)
	assert.Equal(t, 5, listener.ReportConfig.Threshold)
	assert.Equal(t, 6, listener.ReportConfig.AutoBanThreshold)
}

func TestRuleSetServiceUpdateAppliesRuntimeWithoutRestart(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	var opts options
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db"))
	opts.Files.SamplesDataPath = tmpDir
	opts.Files.DynamicDataPath = tmpDir
	opts.Moderation.FirstStrike = 30 * time.Minute
	opts.Moderation.SecondStrike = 6 * time.Hour
	opts.Moderation.WarnDeleteDuration = time.Minute
	opts.Report.Threshold = 2
	opts.Report.AutoBanThreshold = 3
	opts.Meta.LinksLimit = 1
	opts.Duplicates.Threshold = 2
	opts.Duplicates.Window = time.Minute
	opts.Dry = false
	opts.SoftBan = false
	t.Setenv("DRY", "false")
	t.Setenv("SOFT_BAN", "false")

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	tbAPI := &tbapi.BotAPI{Self: tbapi.User{UserName: "bot"}}
	listener := assembly.makeTelegramListener(opts, tbAPI)
	assembly.wireLiveReload(opts)

	require.Equal(t, 1, assembly.ActiveRuleSet.Version)
	assert.Equal(t, 30*time.Minute, listener.ModerationConfig.FirstStrike)
	assert.Equal(t, time.Minute, listener.ModerationConfig.WarnDeleteDuration)
	assert.False(t, listener.Dry)
	assert.Equal(t, 2, listener.ReportConfig.Threshold)
	assert.Equal(t, 2, assembly.Detector.DuplicateDetection.Threshold)

	updated, err := assembly.RuleSetService.Update(ctx, opts.InstanceID, "api", rules.RuleSet{
		Meta: rules.MetaRules{LinksLimit: 7},
		Duplicates: rules.DuplicateRules{
			Threshold: 9,
			Window:    2 * time.Minute,
		},
		Moderation: rules.ModerationRules{
			FirstStrike:  10 * time.Minute,
			SecondStrike: time.Hour,
			SoftBan:      true,
			DryRun:       true,
		},
		Reports: rules.ReportRules{
			Enabled:          true,
			Threshold:        5,
			AutoBanThreshold: 6,
			RateLimit:        4,
			RatePeriod:       2 * time.Minute,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, updated.Version)
	assert.Equal(t, 2, assembly.ActiveRuleSet.Version)
	assert.Equal(t, 10*time.Minute, listener.ModerationConfig.FirstStrike)
	assert.Equal(t, time.Hour, listener.ModerationConfig.SecondStrike)
	assert.False(t, listener.SoftBanMode)
	assert.False(t, listener.Dry)
	assert.Equal(t, 5, listener.ReportConfig.Threshold)
	assert.Equal(t, 6, listener.ReportConfig.AutoBanThreshold)
	assert.Equal(t, 2, listener.RuleSetVersion)
	assert.Equal(t, 9, assembly.Detector.DuplicateDetection.Threshold)
	assert.Equal(t, 2*time.Minute, assembly.Detector.DuplicateDetection.Window)
}

func TestCacheInvalidationOnControlPlaneChanges(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	var opts options
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db"))
	opts.Files.SamplesDataPath = tmpDir
	opts.Files.DynamicDataPath = tmpDir
	opts.Meta.LinksLimit = 1

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	tbAPI := &tbapi.BotAPI{Self: tbapi.User{UserName: "bot"}}
	assembly.makeTelegramListener(opts, tbAPI)
	assembly.wireLiveReload(opts)

	approvedNotified := 0
	assembly.ApprovedUsersService.OnChange(func() { approvedNotified++ })

	dictNotified := 0
	assembly.DictionaryService.OnChange(func() { dictNotified++ })

	spamNotified := 0
	assembly.DetectedSpamService.OnChange(func() { spamNotified++ })

	require.NoError(t, assembly.ApprovedUsersService.Add(ctx, opts.InstanceID, approved.UserInfo{UserID: "u1", UserName: "user1"}))
	assert.Equal(t, 1, approvedNotified)

	require.NoError(t, assembly.DictionaryService.Add(ctx, opts.InstanceID, storage.DictionaryTypeStopPhrase, "test phrase"))
	assert.Equal(t, 1, dictNotified)

	require.NoError(t, assembly.DetectedSpamService.SetAddedToSamplesFlag(ctx, opts.InstanceID, 999))
	assert.Equal(t, 1, spamNotified)
}
