package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-pkgz/rest"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/genai"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/app/webapi"
	"github.com/umputun/tg-spam/lib/tgspam"
	"github.com/umputun/tg-spam/lib/tgspam/plugin"
)

// makeDB creates database connection based on options
// if dbURL is a file name, uses sqlite with dynamic data path, otherwise uses dbURL as is
func makeDB(ctx context.Context, opts options) (*engine.SQL, error) {
	if opts.DataBaseURL == "" {
		return nil, errors.New("empty database URL")
	}
	dbURL := opts.DataBaseURL // default to what is set in options

	// if dbURL has no path separator, assume it is a file name and add dynamic data path for sqlite
	if !strings.Contains(dbURL, "/") && !strings.Contains(dbURL, "\\") {
		dbURL = filepath.Join(opts.Files.DynamicDataPath, dbURL)
	}
	log.Printf("[DEBUG] data db: %s", dbURL)

	db, err := engine.New(ctx, dbURL, opts.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("can't make db %s, %w", opts.DataBaseURL, err)
	}

	// backup db on version change for sqlite
	if db.Type() == engine.Sqlite {
		// get file name from dbURL for sqlite
		dbFile := dbURL
		dbFile = strings.TrimPrefix(dbFile, "file://")
		dbFile = strings.TrimPrefix(dbFile, "file:")

		// make backup of db on version change for sqlite
		if opts.MaxBackups > 0 {
			if err := backupDB(dbFile, revision, opts.MaxBackups); err != nil {
				return nil, fmt.Errorf("backup on version change failed, %w", err)
			}
		} else {
			log.Print("[WARN] database backups disabled")
		}
	}
	return db, nil
}

// checkVolumeMount checks if dynamic files location mounted in docker and shows warning if not
// returns true if running not in docker or dynamic files dir mounted
func checkVolumeMount(opts options) (ok bool) {
	if os.Getenv("TGSPAM_IN_DOCKER") != "1" {
		return true
	}
	log.Printf("[DEBUG] running in docker")
	warnMsg := fmt.Sprintf("dynamic files dir %q is not mounted, changes will be lost on container restart",
		opts.Files.DynamicDataPath)

	// check if dynamic files dir not present. This means it is not mounted
	_, err := os.Stat(opts.Files.DynamicDataPath)
	if err != nil {
		log.Printf("[WARN] %s", warnMsg)
		// no dynamic files dir, no need to check further
		return false
	}

	// check if .not_mounted file missing, this means it is mounted
	if _, err = os.Stat(filepath.Join(opts.Files.DynamicDataPath, ".not_mounted")); err != nil {
		return true
	}

	// if .not_mounted file present, it can be mounted anyway with docker named volumes
	output, err := exec.Command("mount").Output()
	if err != nil {
		log.Printf("[WARN] %s, can't check mount: %v", warnMsg, err)
		return true
	}
	// check if the output contains the specified directory
	for line := range strings.SplitSeq(string(output), "\n") {
		if strings.Contains(line, opts.Files.DynamicDataPath) {
			return true
		}
	}

	log.Printf("[WARN] %s", warnMsg)
	return false
}

func activateServer(
	ctx context.Context, opts options, web webRuntimeAssembly, dmUsersProvider webapi.DMUsersProvider,
) (err error) {
	authPassswd := opts.Server.AuthPasswd
	if opts.Server.AuthPasswd == "auto" {
		authPassswd, err = webapi.GenerateRandomPassword(20)
		if err != nil {
			return fmt.Errorf("can't generate random password, %w", err)
		}
		authHash, err := rest.GenerateBcryptHash(authPassswd)
		if err != nil {
			return fmt.Errorf("can't generate bcrypt hash for password, %w", err)
		}
		log.Printf("[WARN] generated basic auth password for user tg-spam: %q, bcrypt hash: %s", authPassswd, authHash)
	}

	// make store and load approved users
	db, ok := web.StorageEngine.(*engine.SQL)
	if !ok {
		return fmt.Errorf("web storage engine must be *engine.SQL")
	}

	detectedSpamStore, dsErr := storage.NewDetectedSpam(ctx, db)
	if dsErr != nil {
		return fmt.Errorf("can't make detected spam store, %w", dsErr)
	}

	// make dictionary store for webapi
	dictionaryStore, dictErr := storage.NewDictionary(ctx, db)
	if dictErr != nil {
		return fmt.Errorf("can't make dictionary store, %w", dictErr)
	}

	settings := webapi.Settings{
		TenantID:                opts.InstanceID,
		BotUsername:             web.BotUsername,
		PrimaryGroup:            opts.Telegram.Group,
		AdminGroup:              opts.AdminGroup,
		DisableAdminSpamForward: opts.DisableAdminSpamForward,
		LoggerEnabled:           opts.Logger.Enabled,
		SuperUsers:              opts.SuperUsers,
		StorageTimeout:          opts.StorageTimeout,
		NoSpamReply:             opts.NoSpamReply,
		CasEnabled:              opts.CAS.API != "",
		MetaEnabled: opts.Meta.ImageOnly || opts.Meta.LinksLimit >= 0 || opts.Meta.MentionsLimit >= 0 ||
			opts.Meta.LinksOnly || opts.Meta.VideosOnly || opts.Meta.AudiosOnly || opts.Meta.ContactOnly ||
			opts.Meta.Forward || opts.Meta.Keyboard || opts.Meta.UsernameSymbols != "" || opts.Meta.Giveaway,
		MetaLinksLimit:           opts.Meta.LinksLimit,
		MetaMentionsLimit:        opts.Meta.MentionsLimit,
		MetaLinksOnly:            opts.Meta.LinksOnly,
		MetaImageOnly:            opts.Meta.ImageOnly,
		MetaVideoOnly:            opts.Meta.VideosOnly,
		MetaAudioOnly:            opts.Meta.AudiosOnly,
		MetaForwarded:            opts.Meta.Forward,
		MetaKeyboard:             opts.Meta.Keyboard,
		MetaContactOnly:          opts.Meta.ContactOnly,
		MetaUsernameSymbols:      opts.Meta.UsernameSymbols,
		MetaGiveaway:             opts.Meta.Giveaway,
		MultiLangLimit:           opts.MultiLangWords,
		LLMConsensus:             opts.LLM.Consensus,
		OpenAIEnabled:            opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "",
		OpenAIVeto:               opts.OpenAI.Veto,
		OpenAIHistorySize:        opts.OpenAI.HistorySize,
		OpenAIModel:              opts.OpenAI.Model,
		OpenAICheckShortMessages: opts.OpenAI.CheckShortMessages,
		OpenAICustomPrompts:      opts.OpenAI.CustomPrompts,
		GeminiEnabled:            opts.Gemini.Token != "",
		GeminiVeto:               opts.Gemini.Veto,
		GeminiHistorySize:        opts.Gemini.HistorySize,
		GeminiModel:              opts.Gemini.Model,
		GeminiCheckShortMessages: opts.Gemini.CheckShortMessages,
		GeminiCustomPrompts:      opts.Gemini.CustomPrompts,
		LuaPluginsEnabled:        opts.LuaPlugins.Enabled,
		LuaPluginsDir:            opts.LuaPlugins.PluginsDir,
		LuaEnabledPlugins:        opts.LuaPlugins.EnabledPlugins,
		LuaDynamicReload:         opts.LuaPlugins.DynamicReload,
		SamplesDataPath:          opts.Files.SamplesDataPath,
		DynamicDataPath:          opts.Files.DynamicDataPath,
		WatchIntervalSecs:        int(opts.Files.WatchInterval.Seconds()),
		SimilarityThreshold:      opts.SimilarityThreshold,
		MinMsgLen:                opts.MinMsgLen,
		MaxEmoji:                 opts.MaxEmoji,
		MinSpamProbability:       opts.MinSpamProbability,
		ParanoidMode:             opts.ParanoidMode,
		FirstMessagesCount:       opts.FirstMessagesCount,
		StartupMessageEnabled:    opts.Message.Startup != "",
		TrainingEnabled:          opts.Training,
		SoftBanEnabled:           opts.SoftBan,
		AbnormalSpacingEnabled:   opts.AbnormalSpacing.Enabled,
		HistorySize:              opts.HistorySize,
		DebugModeEnabled:         opts.Dbg,
		DryModeEnabled:           opts.Dry,
		TGDebugModeEnabled:       opts.TGDbg,
	}

	srv := webapi.Server{Config: webapi.Config{
		ListenAddr:            opts.Server.ListenAddr,
		Detector:              web.Detector,
		SpamFilter:            web.SpamFilter,
		Locator:               web.Locator,
		DetectedSpamStore:     detectedSpamStoreAdapter{inner: detectedSpamStore},
		DetectedSpamProvider:  web.DetectedSpamService,
		DictionaryStore:       dictionaryFallbackAdapter{inner: dictionaryStore},
		DictionaryProvider:    web.DictionaryService,
		StorageEngine:         db,
		DMUsersProvider:       dmUsersProvider,
		RuleSetProvider:       web.RuleSetService,
		ControlPlaneAuth:      web.RoleAuthorizer,
		ApprovedUsersProvider: web.ApprovedUsersService,
		RateLimiter:           webapi.NewTenantRateLimiter(50, 50),
		TenantStatusProvider:  web.TenantStatusProvider,
		AuditService:          web.AuditService,
		AppealService:         web.AppealService,
		FeedbackService:       web.FeedbackService,
		ReviewService:         web.ReviewService,
		KnowledgeService:      web.KnowledgeService,
		OnboardingProvider:    web.OnboardingProvider,
		RestoreProvider:       web.RestoreProvider,
		MetricsCollector:      web.Metrics,
		AuthPasswd:            authPassswd,
		AuthHash:              opts.Server.AuthHash,
		Version:               revision,
		Dbg:                   opts.Dbg,
		Settings:              settings,
	}}

	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("[ERROR] web server failed, %v", err)
		}
	}()
	return nil
}

// makeDetector creates spam detector with all checkers and updaters
// it loads samples and dynamic files
func makeDetector(opts options) *tgspam.Detector {
	return makeDetectorWithRuleSet(opts, bootstrapRuleSet(opts))
}

func makeDetectorWithRuleSet(opts options, ruleSet rules.RuleSet) *tgspam.Detector {
	detectorConfig := buildDetectorConfig(opts, ruleSet)
	detector := tgspam.NewDetector(detectorConfig)

	applyLLMCheckers(detector, opts, ruleSet)

	detector.WithMetaChecks(buildMetaChecks(ruleSet, ruleSet.Detection.MinMsgLen)...)
	debugLogFields("detector config", detectorConfig)

	// initialize Lua plugins if enabled
	if opts.LuaPlugins.Enabled {
		detector.LuaPlugins.Enabled = true
		detector.LuaPlugins.PluginsDir = opts.LuaPlugins.PluginsDir
		detector.LuaPlugins.EnabledPlugins = opts.LuaPlugins.EnabledPlugins
		detector.LuaPlugins.DynamicReload = opts.LuaPlugins.DynamicReload

		luaEngine := plugin.NewChecker()
		if err := detector.WithLuaEngine(luaEngine); err != nil {
			log.Printf("[WARN] failed to initialize Lua plugins: %v", err)
		} else {
			log.Printf("[INFO] lua plugins enabled from directory: %s", opts.LuaPlugins.PluginsDir)
			if len(opts.LuaPlugins.EnabledPlugins) > 0 {
				log.Printf("[INFO] enabled Lua plugins: %v", opts.LuaPlugins.EnabledPlugins)
			} else {
				log.Print("[INFO] all Lua plugins from directory are enabled")
			}

			if opts.LuaPlugins.DynamicReload {
				log.Print("[INFO] dynamic reloading of Lua plugins enabled")
			}
		}
	}

	return detector
}

// applyLLMCheckers (re)builds and attaches the OpenAI and Gemini text checkers on the
// detector from the current ruleset. Safe to call repeatedly for live reload.
func applyLLMCheckers(detector *tgspam.Detector, opts options, ruleSet rules.RuleSet) {
	if ruleSet.OpenAI.Enabled && (opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "") {
		openAIConfig := tgspam.OpenAIConfig{
			SystemPrompt:                 ruleSet.OpenAI.Prompt,
			CustomPrompts:                ruleSet.OpenAI.CustomPrompts,
			Model:                        ruleSet.OpenAI.Model,
			MaxTokensResponse:            opts.OpenAI.MaxTokensResponse,
			MaxTokensRequest:             opts.OpenAI.MaxTokensRequest,
			MaxSymbolsRequest:            opts.OpenAI.MaxSymbolsRequest,
			RetryCount:                   opts.OpenAI.RetryCount,
			ReasoningEffort:              opts.OpenAI.ReasoningEffort,
			CheckShortMessagesWithOpenAI: ruleSet.OpenAI.CheckShortMessages,
		}
		config := openai.DefaultConfig(opts.OpenAI.Token)
		if opts.OpenAI.APIBase != "" {
			config.BaseURL = opts.OpenAI.APIBase
		}
		debugLogFields("openai config", openAIConfig)
		detector.WithOpenAIChecker(openai.NewClientWithConfig(config), openAIConfig)
	}

	if ruleSet.Gemini.Enabled && opts.Gemini.Token != "" {
		geminiConfig := tgspam.GeminiConfig{
			SystemPrompt:       ruleSet.Gemini.Prompt,
			CustomPrompts:      ruleSet.Gemini.CustomPrompts,
			Model:              ruleSet.Gemini.Model,
			MaxOutputTokens:    opts.Gemini.MaxTokensResponse,
			MaxSymbolsRequest:  opts.Gemini.MaxSymbolsRequest,
			RetryCount:         opts.Gemini.RetryCount,
			CheckShortMessages: ruleSet.Gemini.CheckShortMessages,
		}
		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:  opts.Gemini.Token,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Printf("[ERROR] failed to create gemini client: %v", err)
			return
		}
		debugLogFields("gemini config", geminiConfig)
		detector.WithGeminiChecker(client.Models, geminiConfig)
	}
}

func buildDetectorConfig(opts options, ruleSet rules.RuleSet) tgspam.Config {
	casAPI := ""
	if ruleSet.Detection.CasEnabled {
		casAPI = opts.CAS.API
	}
	cfg := tgspam.Config{
		MaxAllowedEmoji:     ruleSet.Detection.MaxEmoji,
		MinMsgLen:           ruleSet.Detection.MinMsgLen,
		SimilarityThreshold: ruleSet.Detection.SimilarityThreshold,
		MinSpamProbability:  ruleSet.Detection.MinSpamProbability,
		CasAPI:              casAPI,
		CasUserAgent:        opts.CAS.UserAgent,
		HTTPClient:          &http.Client{Timeout: opts.CAS.Timeout},
		FirstMessageOnly:    !ruleSet.Detection.ParanoidMode,
		FirstMessagesCount:  ruleSet.Detection.FirstMessagesCount,
		OpenAIVeto:          ruleSet.OpenAI.Veto,
		OpenAIHistorySize:   ruleSet.OpenAI.HistorySize,
		GeminiVeto:          ruleSet.Gemini.Veto,
		GeminiHistorySize:   ruleSet.Gemini.HistorySize,
		LLMMode:             tgspam.LLMMode(ruleSet.LLM.Mode),
		LLMConsensus:        tgspam.LLMConsensusMode(ruleSet.LLM.Consensus),
		LLMRequestTimeout:   opts.LLM.RequestTimeout,
		MultiLangWords:      ruleSet.Detection.MultiLangWords,
		HistorySize:         ruleSet.Detection.HistorySize,
	}

	if ruleSet.Detection.FirstMessagesCount > 0 {
		cfg.FirstMessageOnly = true
	}
	if ruleSet.Detection.ParanoidMode {
		cfg.FirstMessageOnly = false
		cfg.FirstMessagesCount = 0
	}
	if opts.StorageTimeout > 0 {
		cfg.StorageTimeout = opts.StorageTimeout
	}

	cfg.DuplicateDetection.Threshold = ruleSet.Duplicates.Threshold
	cfg.DuplicateDetection.Window = ruleSet.Duplicates.Window

	cfg.AbnormalSpacing.Enabled = ruleSet.AbnormalSpacing.Enabled
	cfg.AbnormalSpacing.ShortWordLen = ruleSet.AbnormalSpacing.ShortWordLen
	cfg.AbnormalSpacing.ShortWordRatioThreshold = ruleSet.AbnormalSpacing.ShortWordRatioThreshold
	cfg.AbnormalSpacing.SpaceRatioThreshold = ruleSet.AbnormalSpacing.SpaceRatioThreshold
	cfg.AbnormalSpacing.MinWordsCount = ruleSet.AbnormalSpacing.MinWords

	return cfg
}

func buildMetaChecks(ruleSet rules.RuleSet, minMsgLen int) []tgspam.MetaCheck {
	var mc []tgspam.MetaCheck
	if ruleSet.Meta.ImageOnly {
		mc = append(mc, tgspam.ImagesCheck(minMsgLen))
	}
	if ruleSet.Meta.VideosOnly {
		mc = append(mc, tgspam.VideosCheck(minMsgLen))
	}
	if ruleSet.Meta.AudiosOnly {
		mc = append(mc, tgspam.AudioCheck(minMsgLen))
	}
	if ruleSet.Meta.LinksLimit >= 0 {
		mc = append(mc, tgspam.LinksCheck(ruleSet.Meta.LinksLimit))
	}
	if ruleSet.Meta.MentionsLimit >= 0 {
		mc = append(mc, tgspam.MentionsCheck(ruleSet.Meta.MentionsLimit))
	}
	if ruleSet.Meta.LinksOnly {
		mc = append(mc, tgspam.LinkOnlyCheck())
	}
	if ruleSet.Meta.Forwarded {
		mc = append(mc, tgspam.ForwardedCheck())
	}
	if ruleSet.Meta.Keyboard {
		mc = append(mc, tgspam.KeyboardCheck())
	}
	if ruleSet.Meta.ContactOnly {
		mc = append(mc, tgspam.ContactCheck())
	}
	if ruleSet.Meta.UsernameSymbols != "" {
		mc = append(mc, tgspam.UsernameSymbolsCheck(ruleSet.Meta.UsernameSymbols))
	}
	if ruleSet.Meta.Giveaway {
		mc = append(mc, tgspam.GiveawayCheck())
	}
	return mc
}

func makeSpamBot(
	ctx context.Context, opts options, ruleSet rules.RuleSet, dataDB *engine.SQL, detector *tgspam.Detector,
) (*bot.SpamFilter, error) {
	if dataDB == nil || detector == nil {
		return nil, errors.New("nil datadb or detector")
	}

	// make samples store
	samplesStore, err := storage.NewSamples(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make samples store, %w", err)
	}
	if err = migrateSamples(ctx, opts, samplesStore); err != nil {
		return nil, fmt.Errorf("can't migrate samples, %w", err)
	}

	// make dictionary store
	dictionaryStore, err := storage.NewDictionary(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make dictionary store, %w", err)
	}
	if err := migrateDicts(ctx, opts, dictionaryStore); err != nil {
		return nil, fmt.Errorf("can't migrate dictionary, %w", err)
	}

	spamBotParams := bot.SpamConfig{
		TenantID:     opts.InstanceID,
		SamplesStore: samplesStore,
		DictStore:    dictionaryStore,
		SpamMsg:      opts.Message.Spam,
		SpamDryMsg:   opts.Message.Dry,
		Dry:          ruleSet.Moderation.DryRun,
	}
	spamBot := bot.NewSpamFilter(detector, spamBotParams)
	debugLogFields("spam bot config", spamBotParams)

	if err := spamBot.ReloadSamples(ctx); err != nil {
		return nil, fmt.Errorf("can't reload samples, %w", err)
	}

	// set detector samples updaters
	detector.WithSpamUpdater(storage.NewSampleUpdater(samplesStore, storage.SampleTypeSpam, opts.StorageTimeout))
	detector.WithHamUpdater(storage.NewSampleUpdater(samplesStore, storage.SampleTypeHam, opts.StorageTimeout))

	return spamBot, nil
}

// expandPath expands ~ to home dir and makes the absolute path
func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, path[1:])
	}
	ep, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return ep
}

func makeSlowPathEngine(opts options, ruleSet rules.RuleSet) *slowpath.Engine {
	hasProvider := opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "" || opts.Gemini.Token != ""
	if !hasProvider {
		return nil
	}

	eng := slowpath.NewEngine(slowpath.EngineConfig{})
	brk := slowpath.DefaultBreakerConfig()
	applySlowPathPrompts(eng, ruleSet)

	if opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "" {
		config := openai.DefaultConfig(opts.OpenAI.Token)
		if opts.OpenAI.APIBase != "" {
			config.BaseURL = opts.OpenAI.APIBase
		}
		oaClient := openai.NewClientWithConfig(config)
		textAdapter := slowpath.NewOpenAIAdapter(oaClient, opts.OpenAI.Model,
			opts.OpenAI.MaxTokensResponse, opts.OpenAI.MaxSymbolsRequest)
		eng.RegisterProvider(textAdapter, brk)

		if vm := opts.OpenAI.VisionModel; vm != "" {
			visionAdapter := slowpath.NewOpenAIAdapter(oaClient, vm, opts.OpenAI.MaxTokensResponse, opts.OpenAI.MaxSymbolsRequest)
			eng.RegisterVision(visionAdapter, brk)
			log.Printf("[INFO] slowpath openai registered (text: %s, vision: %s)", opts.OpenAI.Model, vm)
		} else {
			eng.RegisterVision(textAdapter, brk)
			log.Printf("[INFO] slowpath openai registered (text+vision: %s)", opts.OpenAI.Model)
		}
	}

	if opts.Gemini.Token != "" {
		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:  opts.Gemini.Token,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Printf("[WARN] slowpath gemini client failed: %v", err)
		} else {
			adapter := slowpath.NewGeminiAdapter(client.Models, opts.Gemini.Model, slowpath.GeminiAdapterConfig{
				MaxOutputTokens:   opts.Gemini.MaxTokensResponse,
				MaxSymbolsRequest: opts.Gemini.MaxSymbolsRequest,
			})
			eng.RegisterProvider(adapter, brk)
			eng.RegisterVision(adapter, brk)
			log.Printf("[INFO] slowpath gemini registered (text+vision)")
		}
	}

	return eng
}

func makeSlowPathChatEngine(opts options) *slowpath.Engine {
	if opts.ChatModel == "" {
		return nil
	}
	if opts.OpenAI.Token == "" && opts.OpenAI.APIBase == "" {
		return nil
	}

	config := openai.DefaultConfig(opts.OpenAI.Token)
	if opts.OpenAI.APIBase != "" {
		config.BaseURL = opts.OpenAI.APIBase
	}

	eng := slowpath.NewEngine(slowpath.EngineConfig{})
	chatAdapter := slowpath.NewOpenAIAdapter(openai.NewClientWithConfig(config), opts.ChatModel,
		opts.OpenAI.MaxTokensResponse, opts.OpenAI.MaxSymbolsRequest)
	eng.RegisterChat(chatAdapter, slowpath.DefaultBreakerConfig())
	log.Printf("[INFO] slowpath openai chat registered (%s)", opts.ChatModel)
	return eng
}

func applySlowPathPrompts(eng *slowpath.Engine, ruleSet rules.RuleSet) {
	if eng == nil {
		return
	}
	eng.SetSystemPrompt("openai", ruleSet.OpenAI.Prompt)
	eng.SetSystemPrompt("gemini", ruleSet.Gemini.Prompt)
	eng.SetVisionPrompt(ruleSet.LLM.VisionPrompt)
}
