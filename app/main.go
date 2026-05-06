package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/tg-spam/app/events"
)

type options struct {
	InstanceID  string `long:"instance-id" env:"INSTANCE_ID" default:"tg-spam" description:"instance id"`
	DataBaseURL string `long:"db" env:"DB" default:"tg-spam.db" description:"database URL, if empty uses sqlite"`

	Telegram struct {
		Token        string        `long:"token" env:"TOKEN" description:"telegram bot token"`
		Group        string        `long:"group" env:"GROUP" description:"group name/id"`
		Timeout      time.Duration `long:"timeout" env:"TIMEOUT" default:"30s" description:"http client timeout for telegram" `
		IdleDuration time.Duration `long:"idle" env:"IDLE" default:"30s" description:"idle duration"`
	} `group:"telegram" namespace:"telegram" env-namespace:"TELEGRAM"`

	AdminGroup              string `long:"admin.group" env:"ADMIN_GROUP" description:"admin group name, or channel id"`
	DisableAdminSpamForward bool   `long:"disable-admin-spam-forward" env:"DISABLE_ADMIN_SPAM_FORWARD" description:"disable handling messages forwarded to admin group as spam"`

	TestingIDs []int64 `long:"testing-id" env:"TESTING_ID" env-delim:"," description:"testing ids, allow bot to reply to them"`

	HistoryDuration time.Duration `long:"history-duration" env:"HISTORY_DURATION" default:"24h" description:"history duration"`
	HistoryMinSize  int           `long:"history-min-size" env:"HISTORY_MIN_SIZE" default:"1000" description:"history minimal size to keep"`
	StorageTimeout  time.Duration `long:"storage-timeout" env:"STORAGE_TIMEOUT" default:"0s" description:"storage timeout"`

	Logger struct {
		Enabled    bool   `long:"enabled" env:"ENABLED" description:"enable spam rotated logs"`
		FileName   string `long:"file" env:"FILE" default:"tg-spam.log" description:"location of spam log"`
		MaxSize    string `long:"max-size" env:"MAX_SIZE" default:"100M" description:"maximum size before it gets rotated"`
		MaxBackups int    `long:"max-backups" env:"MAX_BACKUPS" default:"10" description:"maximum number of old log files to retain"`
	} `group:"logger" namespace:"logger" env-namespace:"LOGGER"`

	SuperUsers          events.SuperUsers `long:"super" env:"SUPER_USER" env-delim:"," description:"super-users"`
	NoSpamReply         bool              `long:"no-spam-reply" env:"NO_SPAM_REPLY" description:"do not reply to spam messages"`
	SuppressJoinMessage bool              `long:"suppress-join-message" env:"SUPPRESS_JOIN_MESSAGE" description:"delete join message if user is kicked out"`

	Delete struct {
		JoinMessages  bool `long:"join-messages" env:"JOIN_MESSAGES" description:"delete join messages immediately"`
		LeaveMessages bool `long:"leave-messages" env:"LEAVE_MESSAGES" description:"delete leave messages immediately"`
	} `group:"delete" namespace:"delete" env-namespace:"DELETE"`

	CAS struct {
		API       string        `long:"api" env:"API" default:"https://api.cas.chat" description:"CAS API"`
		Timeout   time.Duration `long:"timeout" env:"TIMEOUT" default:"5s" description:"CAS timeout"`
		UserAgent string        `long:"user-agent" env:"USER_AGENT" description:"User-Agent header for CAS API requests"`
	} `group:"cas" namespace:"cas" env-namespace:"CAS"`

	Meta struct {
		LinksLimit      int    `long:"links-limit" env:"LINKS_LIMIT" default:"-1" description:"max links in message, disabled by default"`
		MentionsLimit   int    `long:"mentions-limit" env:"MENTIONS_LIMIT" default:"-1" description:"max mentions in message, disabled by default"`
		ImageOnly       bool   `long:"image-only" env:"IMAGE_ONLY" description:"enable image only check"`
		LinksOnly       bool   `long:"links-only" env:"LINKS_ONLY" description:"enable links only check"`
		VideosOnly      bool   `long:"video-only" env:"VIDEO_ONLY" description:"enable video only check"`
		AudiosOnly      bool   `long:"audio-only" env:"AUDIO_ONLY" description:"enable audio only check"`
		ContactOnly     bool   `long:"contact-only" env:"CONTACT_ONLY" description:"enable contact only check"`
		Forward         bool   `long:"forward" env:"FORWARD" description:"enable forward check"`
		Keyboard        bool   `long:"keyboard" env:"KEYBOARD" description:"enable keyboard check"`
		UsernameSymbols string `long:"username-symbols" env:"USERNAME_SYMBOLS" description:"prohibited symbols in username, disabled by default"`
		Giveaway        bool   `long:"giveaway" env:"GIVEAWAY" description:"enable giveaway check"`
	} `group:"meta" namespace:"meta" env-namespace:"META"`

	OpenAI struct {
		Token              string   `long:"token" env:"TOKEN" description:"openai token, disabled if not set"`
		APIBase            string   `long:"apibase" env:"API_BASE" description:"custom openai API base, default is https://api.openai.com/v1"`
		Veto               bool     `long:"veto" env:"VETO" description:"veto mode, confirm detected spam"`
		Prompt             string   `long:"prompt" env:"PROMPT" default:"" description:"openai system prompt, if empty uses builtin default"`
		CustomPrompts      []string `long:"custom-prompt" env:"CUSTOM_PROMPT" env-delim:"," description:"additional custom prompts for specific spam patterns"`
		Model              string   `long:"model" env:"MODEL" default:"gpt-4o-mini" description:"openai model"`
		VisionModel        string   `long:"vision-model" env:"VISION_MODEL" default:"" description:"openai model for vision/image analysis, uses --openai.model if empty"`
		MaxTokensResponse  int      `long:"max-tokens-response" env:"MAX_TOKENS_RESPONSE" default:"1024" description:"openai max tokens in response"`
		MaxTokensRequest   int      `long:"max-tokens-request" env:"MAX_TOKENS_REQUEST" default:"2048" description:"openai max tokens in request"`
		MaxSymbolsRequest  int      `long:"max-symbols-request" env:"MAX_SYMBOLS_REQUEST" default:"16000" description:"openai max symbols in request, failback if tokenizer failed"`
		RetryCount         int      `long:"retry-count" env:"RETRY_COUNT" default:"1" description:"openai retry count"`
		HistorySize        int      `long:"history-size" env:"HISTORY_SIZE" default:"0" description:"openai history size"`
		ReasoningEffort    string   `long:"reasoning-effort" env:"REASONING_EFFORT" default:"none" choice:"none" choice:"low" choice:"medium" choice:"high" description:"reasoning effort for thinking models, none disables thinking"`
		CheckShortMessages bool     `long:"check-short-messages" env:"CHECK_SHORT_MESSAGES" description:"check messages shorter than min-msg-len with OpenAI"`
	} `group:"openai" namespace:"openai" env-namespace:"OPENAI"`

	Gemini struct {
		Token              string   `long:"token" env:"TOKEN" description:"gemini token, disabled if not set"`
		Veto               bool     `long:"veto" env:"VETO" description:"veto mode, confirm detected spam"`
		Prompt             string   `long:"prompt" env:"PROMPT" default:"" description:"gemini system prompt, if empty uses builtin default"`
		CustomPrompts      []string `long:"custom-prompt" env:"CUSTOM_PROMPT" env-delim:"," description:"additional custom prompts for specific spam patterns"`
		Model              string   `long:"model" env:"MODEL" default:"gemma-4-31b-it" description:"gemini model"`
		VisionModel        string   `long:"vision-model" env:"VISION_MODEL" default:"" description:"gemini model for vision/image analysis, defaults to model if not set"`
		MaxTokensResponse  int32    `long:"max-tokens-response" env:"MAX_TOKENS_RESPONSE" default:"1024" description:"gemini max tokens in response"`
		MaxSymbolsRequest  int      `long:"max-symbols-request" env:"MAX_SYMBOLS_REQUEST" default:"8192" description:"gemini max symbols in request"`
		RetryCount         int      `long:"retry-count" env:"RETRY_COUNT" default:"1" description:"gemini retry count"`
		HistorySize        int      `long:"history-size" env:"HISTORY_SIZE" default:"0" description:"gemini history size"`
		CheckShortMessages bool     `long:"check-short-messages" env:"CHECK_SHORT_MESSAGES" description:"check messages shorter than min-msg-len with Gemini"`
	} `group:"gemini" namespace:"gemini" env-namespace:"GEMINI"`

	LLM struct {
		Consensus      string        `long:"consensus" env:"CONSENSUS" choice:"any" choice:"all" default:"any" description:"how eligible LLMs flip the base decision"`
		RequestTimeout time.Duration `long:"request-timeout" env:"REQUEST_TIMEOUT" default:"30s" description:"timeout for individual LLM requests"`
	} `group:"llm" namespace:"llm" env-namespace:"LLM"`

	LuaPlugins struct {
		Enabled        bool     `long:"enabled" env:"ENABLED" description:"enable Lua plugins"`
		PluginsDir     string   `long:"plugins-dir" env:"PLUGINS_DIR" description:"directory with Lua plugins"`
		EnabledPlugins []string `long:"enabled-plugins" env:"ENABLED_PLUGINS" env-delim:"," description:"list of enabled plugins (by name, without .lua extension)"`
		DynamicReload  bool     `long:"dynamic-reload" env:"DYNAMIC_RELOAD" description:"dynamically reload plugins when they change"`
	} `group:"lua-plugins" namespace:"lua-plugins" env-namespace:"LUA_PLUGINS"`

	AbnormalSpacing struct {
		Enabled                 bool    `long:"enabled" env:"ENABLED" description:"enable abnormal words check"`
		SpaceRatioThreshold     float64 `long:"ratio" env:"RATIO" default:"0.3" description:"the ratio of spaces to all characters in the message"`
		ShortWordRatioThreshold float64 `long:"short-ratio" env:"SHORT_RATIO" default:"0.7" description:"the ratio of short words to all words in the message"`
		ShortWordLen            int     `long:"short-word" env:"SHORT_WORD" default:"3" description:"the length of the word to be considered short"`
		MinWords                int     `long:"min-words" env:"MIN_WORDS" default:"5" description:"the minimum number of words in the message to check"`
	} `group:"space" namespace:"space" env-namespace:"SPACE"`

	Duplicates struct {
		Threshold int           `long:"threshold" env:"THRESHOLD" default:"0" description:"duplicate messages to trigger spam (0=disabled)"`
		Window    time.Duration `long:"window" env:"WINDOW" default:"1h" description:"time window for duplicate detection"`
	} `group:"duplicates" namespace:"duplicates" env-namespace:"DUPLICATES"`

	Moderation struct {
		FirstStrike        time.Duration `long:"first-strike" env:"FIRST_STRIKE" default:"30m" description:"mute/restrict duration for the first automatic spam strike"`
		SecondStrike       time.Duration `long:"second-strike" env:"SECOND_STRIKE" default:"6h" description:"mute/restrict duration for the second automatic spam strike"`
		WarnStrikes        int           `long:"warn-strikes" env:"WARN_STRIKES" default:"0" description:"number of warning-only strikes before first ban (0=disabled)"`
		WarnDeleteDuration time.Duration `long:"warn-delete-duration" env:"WARN_DELETE_DURATION" default:"0" description:"auto-delete warning messages after this duration (0=disabled)"`
	} `group:"moderation" namespace:"moderation" env-namespace:"MODERATION"`

	Report struct {
		Enabled          bool          `long:"enabled" env:"ENABLED" description:"enable user spam reporting"`
		Threshold        int           `long:"threshold" env:"THRESHOLD" default:"2" description:"number of reports to trigger admin notification"`
		AutoBanThreshold int           `long:"auto-ban-threshold" env:"AUTO_BAN_THRESHOLD" default:"0" description:"auto-ban after N reports (0=disabled, must be >= threshold)"`
		RateLimit        int           `long:"rate-limit" env:"RATE_LIMIT" default:"1" description:"max reports per user per period"`
		RatePeriod       time.Duration `long:"rate-period" env:"RATE_PERIOD" default:"1m" description:"rate limit time period"`
	} `group:"report" namespace:"report" env-namespace:"REPORT"`

	Files struct {
		SamplesDataPath string        `long:"samples" env:"SAMPLES" description:"samples data path, defaults to dynamic data path"`
		DynamicDataPath string        `long:"dynamic" env:"DYNAMIC" default:"data" description:"dynamic data path"`
		WatchInterval   time.Duration `long:"watch-interval" env:"WATCH_INTERVAL" default:"5s" description:"watch interval for dynamic files, deprecated"`
	} `group:"files" namespace:"files" env-namespace:"FILES"`

	SimilarityThreshold float64 `long:"similarity-threshold" env:"SIMILARITY_THRESHOLD" default:"0.5" description:"spam threshold"`
	MinMsgLen           int     `long:"min-msg-len" env:"MIN_MSG_LEN" default:"50" description:"min message length to check"`
	MaxEmoji            int     `long:"max-emoji" env:"MAX_EMOJI" default:"2" description:"max emoji count in message, -1 to disable check"`
	MinSpamProbability  float64 `long:"min-probability" env:"MIN_PROBABILITY" default:"50" description:"min spam probability percent to ban"`
	MultiLangWords      int     `long:"multi-lang" env:"MULTI_LANG" default:"0" description:"number of words in different languages to consider as spam"`

	ParanoidMode       bool `long:"paranoid" env:"PARANOID" description:"paranoid mode, check all messages"`
	FirstMessagesCount int  `long:"first-messages-count" env:"FIRST_MESSAGES_COUNT" default:"1" description:"number of first messages to check"`

	AggressiveCleanup      bool `long:"aggressive-cleanup" env:"AGGRESSIVE_CLEANUP" description:"delete all messages from user when banned via /spam command"`
	AggressiveCleanupLimit int  `long:"aggressive-cleanup-limit" env:"AGGRESSIVE_CLEANUP_LIMIT" default:"100" description:"max messages to delete in aggressive cleanup mode"`

	Message struct {
		Startup string `long:"startup" env:"STARTUP" default:"" description:"startup message"`
		Spam    string `long:"spam" env:"SPAM" default:"this is spam" description:"spam message"`
		Dry     string `long:"dry" env:"DRY" default:"this is spam (dry mode)" description:"spam dry message"`
		Warn    string `long:"warn" env:"WARN" default:"" description:"warning message (if empty, uses default with strike info)"`
	} `group:"message" namespace:"message" env-namespace:"MESSAGE"`

	Server struct {
		Enabled         bool   `long:"enabled" env:"ENABLED" description:"enable web server"`
		ListenAddr      string `long:"listen" env:"LISTEN" default:":8080" description:"listen address"`
		ProbeListenAddr string `long:"probe-listen" env:"PROBE_LISTEN" default:"" description:"listen address for runtime health/readiness probes"`
		AuthPasswd      string `long:"auth" env:"AUTH" default:"auto" description:"basic auth password for user 'tg-spam'"`
		AuthHash        string `long:"auth-hash" env:"AUTH_HASH" default:"" description:"basic auth password hash for user 'tg-spam'"`
	} `group:"server" namespace:"server" env-namespace:"SERVER"`

	Training bool `long:"training" env:"TRAINING" description:"training mode, passive spam detection only"`
	SoftBan  bool `long:"soft-ban" env:"SOFT_BAN" description:"soft ban mode, restrict user actions but not ban"`

	HistorySize int    `long:"history-size" env:"LAST_MSGS_HISTORY_SIZE" default:"100" description:"history size"`
	Convert     string `long:"convert" choice:"only" choice:"enabled" choice:"disabled" default:"enabled" description:"convert mode for txt samples and other storage files to DB"`

	MaxBackups int `long:"max-backups" env:"MAX_BACKUPS" default:"10" description:"maximum number of backups to keep, set 0 to disable"`

	Retention struct {
		Enabled              bool          `long:"enabled" env:"ENABLED" description:"enable automatic data retention cleanup"`
		Interval             time.Duration `long:"interval" env:"INTERVAL" default:"1h" description:"how often to run retention cleanup"`
		IncidentsTTL         time.Duration `long:"incidents-ttl" env:"INCIDENTS_TTL" default:"720h" description:"time-to-live for incidents (0=keep forever)"`
		AppealsTTL           time.Duration `long:"appeals-ttl" env:"APPEALS_TTL" default:"720h" description:"time-to-live for appeals (0=keep forever)"`
		DetectedSpamTTL      time.Duration `long:"detected-spam-ttl" env:"DETECTED_SPAM_TTL" default:"720h" description:"time-to-live for detected spam entries (0=keep forever)"`
		IncomingEventsTTL    time.Duration `long:"incoming-events-ttl" env:"INCOMING_EVENTS_TTL" default:"168h" description:"time-to-live for incoming events (0=keep forever)"`
		ModerationActionsTTL time.Duration `long:"moderation-actions-ttl" env:"MODERATION_ACTIONS_TTL" default:"720h" description:"time-to-live for moderation actions (0=keep forever)"`
		LabelsTTL            time.Duration `long:"labels-ttl" env:"LABELS_TTL" default:"720h" description:"time-to-live for feedback labels (0=keep forever)"`
		CandidatesTTL        time.Duration `long:"candidates-ttl" env:"CANDIDATES_TTL" default:"720h" description:"time-to-live for review candidates (0=keep forever)"`
		UsageCountersTTL     time.Duration `long:"usage-counters-ttl" env:"USAGE_COUNTERS_TTL" default:"168h" description:"time-to-live for usage counter windows (0=keep forever)"`
	} `group:"retention" namespace:"retention" env-namespace:"RETENTION"`

	Dry   bool `long:"dry" env:"DRY" description:"dry mode, no bans"`
	Dbg   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	TGDbg bool `long:"tg-dbg" env:"TG_DEBUG" description:"telegram debug mode"`
}

// default file names
const (
	samplesSpamFile   = "spam-samples.txt"
	samplesHamFile    = "ham-samples.txt"
	excludeTokensFile = "exclude-tokens.txt" //nolint:gosec // false positive
	stopWordsFile     = "stop-words.txt"     //nolint:gosec // false positive
	dynamicSpamFile   = "spam-dynamic.txt"
	dynamicHamFile    = "ham-dynamic.txt"
	dataFile          = "tg-spam.db"
)

var revision = "local"

func main() {
	if os.Getenv("GO_FLAGS_COMPLETION") == "" {
		fmt.Printf("tg-spam %s\n", revision)
	}
	var opts options
	p := flags.NewParser(&opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	p.SubcommandsOptional = true
	if _, err := p.Parse(); err != nil {
		if !errors.Is(err.(*flags.Error).Type, flags.ErrHelp) {
			log.Printf("[ERROR] cli error: %v", err)
			os.Exit(1)
		}
		os.Exit(2)
	}

	masked := []string{opts.Telegram.Token, opts.OpenAI.Token, opts.Gemini.Token}
	if opts.Server.AuthPasswd != "auto" && opts.Server.AuthPasswd != "" {
		// auto passwd should not be masked as we print it
		masked = append(masked, opts.Server.AuthPasswd)
	}
	if opts.Server.AuthHash != "" {
		masked = append(masked, opts.Server.AuthHash)
	}

	setupLog(opts.Dbg, masked...)

	debugLogFields("options", opts)

	// validate auto-ban threshold
	if opts.Report.AutoBanThreshold > 0 && opts.Report.AutoBanThreshold < opts.Report.Threshold {
		log.Fatalf("[ERROR] auto-ban-threshold (%d) must be >= threshold (%d) or 0 (disabled)",
			opts.Report.AutoBanThreshold, opts.Report.Threshold)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Printf("[WARN] interrupt signal")
		cancel()
	}()

	// expand, make absolute paths
	opts.Files.DynamicDataPath = expandPath(opts.Files.DynamicDataPath)
	if opts.Files.SamplesDataPath == "" {
		opts.Files.SamplesDataPath = opts.Files.DynamicDataPath
	} else {
		opts.Files.SamplesDataPath = expandPath(opts.Files.SamplesDataPath)
	}

	if err := execute(ctx, opts); err != nil {
		log.Printf("[ERROR] %v", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, opts options) error {
	if opts.Dry {
		log.Print("[WARN] dry mode, no actual bans")
	}

	convertOnly := opts.Convert == "only"
	if !opts.Server.Enabled && !convertOnly && (opts.Telegram.Token == "" || opts.Telegram.Group == "") {
		return errors.New("telegram token and group are required")
	}

	checkVolumeMount(opts) // show warning if dynamic files dir not mounted

	// make samples and dynamic data dirs
	if err := os.MkdirAll(opts.Files.SamplesDataPath, 0o700); err != nil {
		return fmt.Errorf("can't make samples dir, %w", err)
	}

	runtimeProbe := newRuntimeProbe(opts.InstanceID, revision)
	if probeErr := activateRuntimeProbe(ctx, opts.Server.ProbeListenAddr, runtimeProbe); probeErr != nil {
		return fmt.Errorf("can't activate runtime probe server, %w", probeErr)
	}
	defer runtimeProbe.SetReady(false)

	assembly, err := assembleRuntime(ctx, opts)
	if err != nil {
		return err
	}
	defer assembly.close()
	if opts.Convert == "only" {
		return nil
	}

	if assembly.RetentionSvc != nil {
		go assembly.RetentionSvc.Run(ctx)
	}

	// activate web server if enabled, server-only mode (no telegram token)
	if opts.Server.Enabled && (opts.Telegram.Token == "" || opts.Telegram.Group == "") {
		// server starts in background goroutine without DM users provider
		if srvErr := activateWebRuntime(ctx, opts, assembly.Web, nil); srvErr != nil {
			return fmt.Errorf("can't activate web server, %w", srvErr)
		}
		runtimeProbe.SetReady(true)
		log.Printf("[WARN] no telegram token and group set, web server only mode")
		<-ctx.Done()
		return nil
	}

	// make telegram bot
	tbAPI, err := tbapi.NewBotAPI(opts.Telegram.Token)
	if err != nil {
		return fmt.Errorf("can't make telegram bot, %w", err)
	}
	tbAPI.Debug = opts.TGDbg

	tgListener := assembly.makeTelegramListener(opts, tbAPI)
	logListenerConfig(tgListener)
	assembly.wireLiveReload(opts)

	// activate web server if enabled, with DM users provider from the telegram listener
	if opts.Server.Enabled {
		if srvErr := activateWebRuntime(ctx, opts, assembly.Web, tgListener); srvErr != nil {
			return fmt.Errorf("can't activate web server, %w", srvErr)
		}
	}
	runtimeProbe.SetReady(true)

	// run telegram listener and event processor loop
	if err := tgListener.Do(ctx); err != nil { //nolint:staticcheck // do() runs infinite loop, always returns error on exit
		return fmt.Errorf("telegram listener failed, %w", err)
	}
	return nil
}

func setupLog(dbg bool, secrets ...string) {
	logOpts := []lgr.Option{lgr.Format(`{{.DT.Format "15:04:05.000"}} [{{.Level}}] {{.Message}}`), lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{
			lgr.Debug,
			lgr.Format(`{{.DT.Format "15:04:05.000"}} [{{.Level}}] {{.Message}}`),
			lgr.StackTraceOnError,
		}
	}

	colorizer := lgr.Mapper{
		ErrorFunc:  func(s string) string { return color.New(color.FgHiRed).Sprint(s) },
		WarnFunc:   func(s string) string { return color.New(color.FgRed).Sprint(s) },
		InfoFunc:   func(s string) string { return color.New(color.FgYellow).Sprint(s) },
		DebugFunc:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
		CallerFunc: func(s string) string { return color.New(color.FgBlue).Sprint(s) },
		TimeFunc:   func(s string) string { return color.New(color.FgCyan).Sprint(s) },
	}
	logOpts = append(logOpts, lgr.Map(colorizer))

	if len(secrets) > 0 {
		logOpts = append(logOpts, lgr.Secret(secrets...))
	}
	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}
