package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam"
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

	// check that the message is saved to the log file
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
		assert.InDelta(t, float64(123), logEntry["user_id"], 0.001) // json.Unmarshal converts numbers to float64
		assert.Equal(t, "Test message blah blah", logEntry["text"])
	}
	require.NoError(t, scanner.Err())

	// check that the message is saved to the database
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

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "spam-samples.txt"), []byte("spam1\n"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "ham-samples.txt"), []byte("ham1\n"), 0o600))

	first, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	first.close()

	db, err := engine.New(ctx, fmt.Sprintf("sqlite://%s", path.Join(tmpDir, "tg-spam.db")), opts.InstanceID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		"INSERT INTO rule_set_versions (workspace_id, gid, version, source, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		"gr1", opts.InstanceID, 2, "test", `{"workspace_id":"gr1","version":2,"source":"test","meta":{"links_limit":5,"mentions_limit":4},"duplicates":{"threshold":4,"window":60000000000},"abnormal_spacing":{"enabled":true,"space_ratio_threshold":0.25,"short_word_ratio_threshold":0.6,"short_word_len":5,"min_words":8},"moderation":{"first_strike":900000000000,"second_strike":21600000000000,"soft_ban":true,"dry_run":true},"reports":{"enabled":true,"threshold":5,"auto_ban_threshold":6,"rate_limit":2,"rate_period":120000000000},"openai":{"enabled":false,"veto":true,"model":"gpt-active","history_size":3,"check_short_messages":true},"gemini":{"enabled":false,"veto":false,"model":"gemini-active","history_size":7,"check_short_messages":false}}`,
		time.Now().UTC(),
	)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		"UPDATE rule_sets SET active_version = ?, updated_at = ? WHERE workspace_id = ? AND gid = ?",
		2, time.Now().UTC(), "gr1", opts.InstanceID,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	assembly, err := assembleRuntime(ctx, opts)
	require.NoError(t, err)
	defer assembly.close()

	assert.Equal(t, 2, assembly.ActiveRuleSet.Version)
	detector, ok := assembly.SpamBot.Detector.(*tgspam.Detector)
	require.True(t, ok)
	assert.Equal(t, 4, detector.DuplicateDetection.Threshold)
	assert.True(t, detector.AbnormalSpacing.Enabled)
	assert.Equal(t, 3, detector.OpenAIHistorySize)

	tbAPI := &tbapi.BotAPI{Self: tbapi.User{UserName: "bot"}}
	listener := assembly.makeTelegramListener(opts, tbAPI)
	assert.Equal(t, 15*time.Minute, listener.ModerationConfig.FirstStrike)
	assert.True(t, listener.SoftBanMode)
	assert.True(t, listener.Dry)
	assert.Equal(t, 5, listener.ReportConfig.Threshold)
	assert.Equal(t, 6, listener.ReportConfig.AutoBanThreshold)
}

func Test_activateServerOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var opts options
	opts.Server.Enabled = true
	opts.Server.ListenAddr = ":9988"
	opts.Server.ProbeListenAddr = ":9989"
	opts.Server.AuthPasswd = "auto"
	opts.InstanceID = "gr1"
	opts.DataBaseURL = fmt.Sprintf("sqlite://%s", path.Join(t.TempDir(), "tg-spam.db"))

	opts.Files.SamplesDataPath, opts.Files.DynamicDataPath = t.TempDir(), t.TempDir()

	// write some sample files
	fh, err := os.Create(path.Join(opts.Files.SamplesDataPath, "spam-samples.txt"))
	require.NoError(t, err)
	_, err = fh.WriteString("spam1\nspam2\nspam3\n")
	require.NoError(t, err)
	fh.Close()

	fh, err = os.Create(path.Join(opts.Files.SamplesDataPath, "ham-samples.txt"))
	require.NoError(t, err)
	_, err = fh.WriteString("ham1\nham2\nham3\n")
	require.NoError(t, err)
	fh.Close()

	done := make(chan struct{})
	go func() {
		execErr := execute(ctx, opts)
		assert.NoError(t, execErr)
		close(done)
	}()

	// wait for server to be ready
	require.Eventually(t, func() bool {
		pingResp, pingErr := http.Get("http://localhost:9988/ping")
		if pingErr != nil {
			return false
		}
		defer pingResp.Body.Close()
		return pingResp.StatusCode == http.StatusOK
	}, time.Second*5, time.Millisecond*100, "server did not start")

	resp, err := http.Get("http://localhost:9988/ping")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))

	healthResp, err := http.Get("http://localhost:9989/healthz")
	require.NoError(t, err)
	defer healthResp.Body.Close()
	assert.Equal(t, http.StatusOK, healthResp.StatusCode)

	readyResp, err := http.Get("http://localhost:9989/readyz")
	require.NoError(t, err)
	defer readyResp.Body.Close()
	assert.Equal(t, http.StatusOK, readyResp.StatusCode)

	cancel()
	<-done
}

func Test_checkVolumeMount(t *testing.T) {
	prepEnvAndFileSystem := func(opts *options, envValue string, dynamicDataPath string, notMountedExists bool) func() {
		os.Setenv("TGSPAM_IN_DOCKER", envValue)

		tempDir, _ := os.MkdirTemp("", "test")
		if dynamicDataPath != "" {
			os.MkdirAll(filepath.Join(tempDir, dynamicDataPath), os.ModePerm)
		}

		if notMountedExists {
			os.WriteFile(filepath.Join(tempDir, dynamicDataPath, ".not_mounted"), []byte{}, 0o644)
		}

		if dynamicDataPath == "" {
			dynamicDataPath = "dynamic"
		}
		opts.Files.DynamicDataPath = filepath.Join(tempDir, dynamicDataPath)

		return func() {
			os.RemoveAll(tempDir)
		}
	}

	tests := []struct {
		name             string
		envValue         string
		dynamicDataPath  string
		notMountedExists bool
		expectedOk       bool
	}{
		{
			name:            "not in docker",
			envValue:        "0",
			dynamicDataPath: "",
			expectedOk:      true,
		},
		{
			name:             "in Docker, path mounted, no .not_mounted",
			envValue:         "1",
			dynamicDataPath:  "dynamic",
			notMountedExists: false,
			expectedOk:       true,
		},
		{
			name:             "in docker, .not_mounted exists",
			envValue:         "1",
			dynamicDataPath:  "dynamic",
			notMountedExists: true,
			expectedOk:       false,
		},
		{
			name:             "not in docker, .not_mounted exists",
			envValue:         "0",
			dynamicDataPath:  "dynamic",
			notMountedExists: true,
			expectedOk:       true,
		},
		{
			name:             "in docker, path not mounted",
			envValue:         "1",
			dynamicDataPath:  "",
			notMountedExists: false,
			expectedOk:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := options{}
			cleanup := prepEnvAndFileSystem(&opts, tt.envValue, tt.dynamicDataPath, tt.notMountedExists)
			defer cleanup()

			ok := checkVolumeMount(opts)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

func Test_expandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	currentDir, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"Empty Path", "", ""},
		{"Home Directory", "~", home},
		{"Relative Path", ".", ""},
		{"Relative Path with directory", "data", filepath.Join(currentDir, "data")},
		{"Absolute Path", "/tmp", "/tmp"},
		{"Path with Tilde and Subdirectory", "~/Documents", filepath.Join(home, "Documents")},
		{"Path with Multiple Relative Directories", "../parent/child", ""},
		{"Path with Special Characters", "data/special @#$/file", ""},
		{"Invalid Path", "/some/nonexistent/path", "/some/nonexistent/path"},
		{"Home Directory with Trailing Slash", "~/", home},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)

			switch {
			case strings.Contains(tt.path, "~"):
				assert.Equal(t, filepath.Join(home, tt.path[1:]), got)
			case tt.path == ".", strings.HasPrefix(tt.path, ".."), strings.Contains(tt.path, "/"):
				// for relative paths, paths starting with "..", and paths with special characters
				expected, err := filepath.Abs(tt.path)
				require.NoError(t, err)
				assert.Equal(t, expected, got)
			default:
				// for absolute paths and invalid paths
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_migrateSamples(t *testing.T) {
	tmpDir := t.TempDir()
	opts := options{}
	opts.Files.SamplesDataPath, opts.Files.DynamicDataPath = tmpDir, tmpDir
	opts.InstanceID = "gr1"

	t.Run("full migration", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		store, err := storage.NewSamples(context.Background(), db)
		require.NoError(t, err)

		// create new files for migration, all 4 files should be migrated
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile),
			[]byte("new spam1\nnew spam2\nnew spam 3"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.DynamicDataPath, samplesHamFile),
			[]byte("new ham1\nnew ham2"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, dynamicSpamFile),
			[]byte("new dspam1\nnew dspam2\nnew dspam3"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile),
			[]byte("new dham1\nnew dham2"), 0o600))

		err = migrateSamples(context.Background(), opts, store)
		require.NoError(t, err)

		// verify all files migrated
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile))
		require.Error(t, err, "original file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.DynamicDataPath, samplesHamFile))
		require.Error(t, err, "original file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, dynamicSpamFile))
		require.Error(t, err, "original file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile))
		require.Error(t, err, "original file should be renamed")

		s, err := store.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 6, s.TotalSpam)
		assert.Equal(t, 4, s.TotalHam)

		res, err := store.Read(context.Background(), storage.SampleTypeSpam, storage.SampleOriginUser)
		require.NoError(t, err)
		assert.Len(t, res, 3)
		assert.Equal(t, "new dspam1", res[0])
		assert.Equal(t, "new dspam2", res[1])
		assert.Equal(t, "new dspam3", res[2])
	})

	t.Run("nil storage", func(t *testing.T) {
		err := migrateSamples(context.Background(), opts, nil)
		assert.Error(t, err)
	})

	t.Run("already migrated", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		store, err := storage.NewSamples(context.Background(), db)
		require.NoError(t, err)

		// create already loaded files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile+".loaded"),
			[]byte("old spam"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile+".loaded"),
			[]byte("old ham"), 0o600))

		err = migrateSamples(context.Background(), opts, store)
		require.NoError(t, err)

		// verify old files untouched
		data, err := os.ReadFile(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile+".loaded"))
		require.NoError(t, err)
		assert.Equal(t, "old spam", string(data))

		// verify new files migrated
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile))
		assert.Error(t, err, "original file should be renamed")
	})

	t.Run("partial migration", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		store, err := storage.NewSamples(context.Background(), db)
		require.NoError(t, err)

		// create mix of loaded and unloaded files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile+".loaded"), []byte("old spam"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile), []byte("new ham"), 0o600))

		err = migrateSamples(context.Background(), opts, store)
		require.NoError(t, err)

		// verify only unloaded files migrated
		_, err = os.Stat(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile))
		require.Error(t, err, "unloaded file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile+".loaded"))
		require.NoError(t, err)

		s, err := store.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, s.TotalSpam)
		assert.Equal(t, 1, s.TotalHam)
	})

	t.Run("empty files", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		store, err := storage.NewSamples(context.Background(), db)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile), []byte(""), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile), []byte(""), 0o600))

		err = migrateSamples(context.Background(), opts, store)
		assert.NoError(t, err)
	})
}

func Test_migrateDicts(t *testing.T) {
	tmpDir := t.TempDir()
	opts := options{}
	opts.Files.SamplesDataPath = tmpDir
	opts.InstanceID = "gr1"

	t.Run("nil dictionary", func(t *testing.T) {
		err := migrateDicts(context.Background(), opts, nil)
		assert.Error(t, err)
	})

	t.Run("full migration", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		dict, err := storage.NewDictionary(context.Background(), db)
		require.NoError(t, err)

		// create new files for migration
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile),
			[]byte("stop1\nstop2\nstop3"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile),
			[]byte("token1\ntoken2"), 0o600))

		err = migrateDicts(context.Background(), opts, dict)
		require.NoError(t, err)

		// verify files renamed and moved correctly
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile))
		require.Error(t, err, "original file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile+".loaded"))
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile))
		require.Error(t, err, "original file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile+".loaded"))
		require.NoError(t, err)

		// verify data imported correctly
		s, err := dict.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 3, s.TotalStopPhrases)
		assert.Equal(t, 2, s.TotalIgnoredWords)
	})

	t.Run("already migrated", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		dict, err := storage.NewDictionary(context.Background(), db)
		require.NoError(t, err)

		// create already loaded files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile+".loaded"),
			[]byte("old stop1\nold stop2"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile+".loaded"),
			[]byte("old token1"), 0o600))

		// create new files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile),
			[]byte("new stop1\nnew stop2"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile),
			[]byte("new token1\nnew token2"), 0o600))

		err = migrateDicts(context.Background(), opts, dict)
		require.NoError(t, err)

		// verify import happened correctly
		s, err := dict.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, s.TotalStopPhrases)
		assert.Equal(t, 2, s.TotalIgnoredWords)

		// verify old files overwritten
		data, err := os.ReadFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile+".loaded"))
		require.NoError(t, err)
		assert.Equal(t, "new stop1\nnew stop2", string(data))
	})

	t.Run("empty files", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		dict, err := storage.NewDictionary(context.Background(), db)
		require.NoError(t, err)

		// create empty files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile), []byte(""), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile), []byte(""), 0o600))

		err = migrateDicts(context.Background(), opts, dict)
		require.NoError(t, err)

		// verify stats
		s, err := dict.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, s.TotalStopPhrases)
		assert.Equal(t, 0, s.TotalIgnoredWords)
	})

	t.Run("partial migration", func(t *testing.T) {
		db, err := engine.NewSqlite(":memory:", "gr1")
		require.NoError(t, err)
		defer db.Close()
		dict, err := storage.NewDictionary(context.Background(), db)
		require.NoError(t, err)

		// create mix of loaded and unloaded files
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, stopWordsFile+".loaded"),
			[]byte("old stop1\nold stop2"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile),
			[]byte("token1\ntoken2"), 0o600))

		err = migrateDicts(context.Background(), opts, dict)
		require.NoError(t, err)

		// verify only unloaded file migrated
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile))
		require.Error(t, err, "unloaded file should be renamed")
		_, err = os.Stat(filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile+".loaded"))
		require.NoError(t, err)

		// verify stats reflect only migrated data
		s, err := dict.Stats(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, s.TotalStopPhrases)
		assert.Equal(t, 2, s.TotalIgnoredWords)
	})
}

func TestBackupDB(t *testing.T) {
	// helper functions
	fileSize := func(t *testing.T, path string) int64 {
		t.Helper()
		info, err := os.Stat(path)
		require.NoError(t, err)
		return info.Size()
	}

	readFile := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(data)
	}

	t.Run("no backup if max 0", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		err := backupDB(dbFile, "v1", 0)
		require.NoError(t, err)
		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("skip existing backup", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		backupFile := dbFile + ".master-123-20250108T00:01:26"
		require.NoError(t, os.WriteFile(backupFile, []byte("old backup"), 0o600))
		origSize := fileSize(t, backupFile)

		err := backupDB(dbFile, "master-123-20250108T00:01:26", 1)
		require.NoError(t, err)

		newSize := fileSize(t, backupFile)
		require.Equal(t, origSize, newSize, "backup file should not be modified")
	})

	t.Run("make new backup and cleanup", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		// create some old backups
		oldBackups := []string{
			"master-111-20250108T00:01:26",
			"master-222-20250108T00:02:26",
			"master-333-20250108T00:03:26",
		}
		for _, v := range oldBackups {
			require.NoError(t, os.WriteFile(dbFile+"."+v, []byte("old"), 0o600))
		}

		// make new backup with maxBackups=2
		newVer := "master-444-20250108T00:04:26"
		err := backupDB(dbFile, newVer, 2)
		require.NoError(t, err)

		// check files
		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Len(t, files, 2)

		// verify correct files remain (2 newest)
		sort.Strings(files) // sort for stable test
		for _, f := range files {
			base := filepath.Base(f)
			require.True(t, strings.HasSuffix(base, oldBackups[2]) || strings.HasSuffix(base, newVer),
				"unexpected file: %s", base)
		}

		content := readFile(t, dbFile+"."+newVer)
		require.Equal(t, "test data", content)
	})

	t.Run("mixed_formats", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		// make older files with version suffix
		require.NoError(t, os.WriteFile(dbFile+".master-aaa-20250101T12:00:00", []byte("1"), 0o600))
		require.NoError(t, os.WriteFile(dbFile+".master-bbb-20250101T13:00:00", []byte("2"), 0o600))

		// make normal files dated between versioned ones
		testTime := time.Date(2025, 1, 1, 12, 30, 0, 0, time.Local)
		require.NoError(t, os.WriteFile(dbFile+".backup1", []byte("3"), 0o600))
		require.NoError(t, os.Chtimes(dbFile+".backup1", testTime, testTime))

		// make new backup, should keep only 3 newest files
		err := backupDB(dbFile, "master-ccc-20250101T14:00:00", 3)
		require.NoError(t, err)

		// check remaining files
		files, err := filepath.Glob(dbFile + ".*")
		require.NoError(t, err)
		require.Len(t, files, 3)

		// verify we have the three newest files by checking their names
		foundFiles := make(map[string]bool)
		for _, f := range files {
			foundFiles[filepath.Base(f)] = true
			t.Logf("found file: %s", filepath.Base(f))
		}

		require.True(t, foundFiles["test.db.master-ccc-20250101T14:00:00"], "newest versioned backup")
		require.True(t, foundFiles["test.db.master-bbb-20250101T13:00:00"], "middle versioned backup")
		require.True(t, foundFiles["test.db.backup1"], "normal backup with mod time in between")

		// and oldest versioned backup should be removed
		_, err = os.Stat(dbFile + ".master-aaa-20250101T12:00:00")
		require.True(t, os.IsNotExist(err), "oldest versioned file should be gone")
	})

	t.Run("version with dots", func(t *testing.T) {
		dir := t.TempDir()
		dbFile := filepath.Join(dir, "test.db")
		require.NoError(t, os.WriteFile(dbFile, []byte("test data"), 0o600))

		version := "master-123-1.2.3-20250108T00:01:26"
		err := backupDB(dbFile, version, 1)
		require.NoError(t, err)

		expectedBackup := dbFile + "." + strings.ReplaceAll(version, ".", "_")
		_, err = os.Stat(expectedBackup)
		require.NoError(t, err)

		content := readFile(t, expectedBackup)
		require.Equal(t, "test data", content)

		require.Contains(t, expectedBackup, "master-123-1_2_3-20250108T00:01:26")
		require.NotContains(t, expectedBackup, "master-123-1.2.3-20250108T00:01:26")
	})

	t.Run("backup with no db file", func(t *testing.T) {
		dir := t.TempDir()
		nonExistentFile := filepath.Join(dir, "non-existent.db")
		err := backupDB(nonExistentFile, "v1", 1)
		require.NoError(t, err)
	})
}
