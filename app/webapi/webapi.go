// Package webapi provides a web API spam detection service.
package webapi

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/go-pkgz/routegroup"

	"github.com/redstone-md/shield/app/audit"
	"github.com/redstone-md/shield/app/events"
	"github.com/redstone-md/shield/app/feedback"
	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/redstone-md/shield/lib/approved"
	"github.com/redstone-md/shield/lib/spamcheck"
)

//go:generate moq --out mocks/detector.go --pkg mocks --with-resets --skip-ensure . Detector
//go:generate moq --out mocks/spam_filter.go --pkg mocks --with-resets --skip-ensure . SpamFilter
//go:generate moq --out mocks/locator.go --pkg mocks --with-resets --skip-ensure . Locator
//go:generate moq --out mocks/detected_spam.go --pkg mocks --with-resets --skip-ensure . DetectedSpam
//go:generate moq --out mocks/storage_engine.go --pkg mocks --with-resets --skip-ensure . StorageEngine
//go:generate moq --out mocks/dictionary.go --pkg mocks --with-resets --skip-ensure . Dictionary
//go:generate moq --out mocks/dm_users_provider.go --pkg mocks --with-resets --skip-ensure . DMUsersProvider

//go:embed assets/* assets/components/*
var templateFS embed.FS
var tmpl = template.Must(template.New("").Funcs(template.FuncMap{"dict": templateDict}).
	ParseFS(templateFS, "assets/*.html", "assets/components/*.html"))

// templateDict builds a map from alternating key/value pairs, for use as a
// html/template FuncMap function so templates can pass named arguments to sub-templates.
func templateDict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict requires an even number of arguments, got %d", len(pairs))
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d is not a string", i)
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// startTime tracks when the server started
var startTime = time.Now()
var requestSeq atomic.Uint64

const (
	requestHeaderEventID       = "X-Event-ID"
	requestHeaderCorrelationID = "X-Correlation-ID"
	requestHeaderRequestID     = "X-Request-ID"
)

// Server is a web API server.
type Server struct {
	Config
}

// Config defines server parameters
type Config struct {
	Version               string                     // version to show in /ping
	ListenAddr            string                     // listen address
	Detector              Detector                   // spam detector
	SpamFilter            SpamFilter                 // spam filter (bot)
	DetectedSpamStore     DetectedSpam               // detected spam storage (fallback when DetectedSpamProvider is nil)
	DetectedSpamProvider  DetectedSpamProvider       // control plane detected spam service
	Locator               Locator                    // locator for user info
	DictionaryStore       Dictionary                 // dictionary storage (fallback when DictionaryProvider is nil)
	DictionaryProvider    DictionaryProvider         // control plane dictionary service (takes precedence over DictionaryStore)
	StorageEngine         StorageEngine              // database engine access for backups
	DMUsersProvider       DMUsersProvider            // provider for recent DM users
	RuleSetProvider       RuleSetProvider            // control plane rule set service
	ControlPlaneAuth      ControlPlaneAuthorizer     // role authorizer for control plane endpoints
	ApprovedUsersProvider ApprovedUsersProvider      // control plane approved users service
	TenantStatusProvider  TenantStatusProvider       // checks if tenant is active (nil = skip check)
	RateLimiter           *TenantRateLimiter         // per-tenant rate limiter (nil = unlimited)
	AuthPasswd            string                     // basic auth password for user "tg-spam"
	AuthHash              string                     // basic auth bcrypt hash for user "tg-spam", takes precedence over AuthPasswd
	AuditService          *audit.Service             // audit service for incidents
	AppealService         *audit.AppealService       // appeal service for incident appeals
	FeedbackService       *feedback.Service          // feedback service for labeling
	ReviewService         *feedback.ReviewService    // review service for candidates
	KnowledgeService      *feedback.KnowledgeService // knowledge snapshot service
	OnboardingProvider    OnboardingService          // tenant onboarding/offboarding
	RestoreProvider       RestoreService             // tenant restore from backup
	MetricsCollector      MetricsProvider            // SLO/SLA metrics
	Dbg                   bool                       // debug mode
	Settings              Settings                   // application settings
	// EnvPinnedKeys lists RuleSet JSON paths whose value is pinned by an env var
	// and will override the stored ruleset on the next restart.
	EnvPinnedKeys map[string]bool
}

type OnboardingService interface {
	Onboard(ctx context.Context, req OnboardRequest) (*OnboardResult, error)
	Offboard(ctx context.Context, tenantID string) error
}

type RestoreService interface {
	RestoreTenant(ctx context.Context, tenantID string, r io.Reader) error
}

type MetricsProvider interface {
	Inc(name string)
	Add(name string, delta int64)
	Observe(name string, duration time.Duration)
	Snapshot() any
}

type OnboardRequest struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	OwnerID  string `json:"owner_id"`
	GID      string `json:"gid"`
}

type OnboardResult struct {
	TenantID    string `json:"tenant_id"`
	WorkspaceID string `json:"workspace_id"`
	RuleSetVer  int    `json:"rule_set_version"`
}

// Settings contains all application settings
type Settings struct {
	TenantID                 string        `json:"tenant_id"`
	BotUsername              string        `json:"bot_username"`
	PrimaryGroup             string        `json:"primary_group"`
	AdminGroup               string        `json:"admin_group"`
	DisableAdminSpamForward  bool          `json:"disable_admin_spam_forward"`
	LoggerEnabled            bool          `json:"logger_enabled"`
	SuperUsers               []string      `json:"super_users"`
	NoSpamReply              bool          `json:"no_spam_reply"`
	CasEnabled               bool          `json:"cas_enabled"`
	MetaEnabled              bool          `json:"meta_enabled"`
	MetaLinksLimit           int           `json:"meta_links_limit"`
	MetaMentionsLimit        int           `json:"meta_mentions_limit"`
	MetaLinksOnly            bool          `json:"meta_links_only"`
	MetaImageOnly            bool          `json:"meta_image_only"`
	MetaVideoOnly            bool          `json:"meta_video_only"`
	MetaAudioOnly            bool          `json:"meta_audio_only"`
	MetaForwarded            bool          `json:"meta_forwarded"`
	MetaKeyboard             bool          `json:"meta_keyboard"`
	MetaContactOnly          bool          `json:"meta_contact_only"`
	MetaUsernameSymbols      string        `json:"meta_username_symbols"`
	MetaGiveaway             bool          `json:"meta_giveaway"`
	MultiLangLimit           int           `json:"multi_lang_limit"`
	LLMConsensus             string        `json:"llm_consensus"`
	OpenAIEnabled            bool          `json:"openai_enabled"`
	OpenAIVeto               bool          `json:"openai_veto"`
	OpenAIHistorySize        int           `json:"openai_history_size"`
	OpenAIModel              string        `json:"openai_model"`
	OpenAICheckShortMessages bool          `json:"openai_check_short_messages"`
	OpenAICustomPrompts      []string      `json:"openai_custom_prompts"`
	GeminiEnabled            bool          `json:"gemini_enabled"`
	GeminiVeto               bool          `json:"gemini_veto"`
	GeminiHistorySize        int           `json:"gemini_history_size"`
	GeminiModel              string        `json:"gemini_model"`
	GeminiCheckShortMessages bool          `json:"gemini_check_short_messages"`
	GeminiCustomPrompts      []string      `json:"gemini_custom_prompts"`
	LuaPluginsEnabled        bool          `json:"lua_plugins_enabled"`
	LuaPluginsDir            string        `json:"lua_plugins_dir"`
	LuaEnabledPlugins        []string      `json:"lua_enabled_plugins"`
	LuaDynamicReload         bool          `json:"lua_dynamic_reload"`
	LuaAvailablePlugins      []string      `json:"lua_available_plugins"` // the list of all available Lua plugins
	SamplesDataPath          string        `json:"samples_data_path"`
	DynamicDataPath          string        `json:"dynamic_data_path"`
	WatchIntervalSecs        int           `json:"watch_interval_secs"`
	SimilarityThreshold      float64       `json:"similarity_threshold"`
	MinMsgLen                int           `json:"min_msg_len"`
	MaxEmoji                 int           `json:"max_emoji"`
	MinSpamProbability       float64       `json:"min_spam_probability"`
	ParanoidMode             bool          `json:"paranoid_mode"`
	FirstMessagesCount       int           `json:"first_messages_count"`
	StartupMessageEnabled    bool          `json:"startup_message_enabled"`
	TrainingEnabled          bool          `json:"training_enabled"`
	StorageTimeout           time.Duration `json:"storage_timeout"`
	SoftBanEnabled           bool          `json:"soft_ban_enabled"`
	AbnormalSpacingEnabled   bool          `json:"abnormal_spacing_enabled"`
	HistorySize              int           `json:"history_size"`
	DebugModeEnabled         bool          `json:"debug_mode_enabled"`
	DryModeEnabled           bool          `json:"dry_mode_enabled"`
	TGDebugModeEnabled       bool          `json:"tg_debug_mode_enabled"`
}

// Detector is a spam detector interface.
type Detector interface {
	Check(req spamcheck.Request) (spam bool, cr []spamcheck.Response)
	ApprovedUsers() []approved.UserInfo
	AddApprovedUser(user approved.UserInfo) error
	RemoveApprovedUser(id string) error
	GetLuaPluginNames() []string // Returns the list of available Lua plugin names
}

// SpamFilter is a spam filter, bot interface.
type SpamFilter interface {
	UpdateSpam(msg string) error
	UpdateHam(msg string) error
	ReloadSamples(ctx context.Context) (err error)
	DynamicSamples(ctx context.Context) (spam, ham []string, err error)
	RemoveDynamicSpamSample(sample string) error
	RemoveDynamicHamSample(sample string) error
}

// Locator is a storage interface used to get user id by name and vice versa.
type Locator interface {
	UserIDByName(ctx context.Context, userName string) int64
	UserNameByID(ctx context.Context, userID int64) string
}

// DetectedSpam is a storage interface used to get detected spam messages and set added flag.
type DetectedSpam interface {
	Read(ctx context.Context, tenantID string) ([]storage.DetectedSpamInfo, error)
	SetAddedToSamplesFlag(ctx context.Context, tenantID string, id int64) error
	FindByUserID(ctx context.Context, tenantID string, userID int64) (*storage.DetectedSpamInfo, error)
}

// DetectedSpamProvider provides access to detected spam through the control plane service layer.
type DetectedSpamProvider interface {
	DetectedSpam
}

// StorageEngine provides access to the database engine for operations like backup
type StorageEngine interface {
	Backup(ctx context.Context, w io.Writer) error
	Type() engine.Type
	BackupSqliteAsPostgres(ctx context.Context, w io.Writer) error
}

// Dictionary is a storage interface for managing stop phrases and ignored words
type Dictionary interface {
	Add(ctx context.Context, tenantID string, t storage.DictionaryType, data string) error
	Delete(ctx context.Context, tenantID string, id int64) error
	Read(ctx context.Context, tenantID string, t storage.DictionaryType) ([]string, error)
	ReadWithIDs(ctx context.Context, tenantID string, t storage.DictionaryType) ([]storage.DictionaryEntry, error)
	Stats(ctx context.Context, tenantID string) (*storage.DictionaryStats, error)
}

// DMUsersProvider provides access to recent DM users for the admin UI
type DMUsersProvider interface {
	GetDMUsers() []events.DMUser
}

// ApprovedUsersProvider provides access to approved users through the control plane service layer.
type ApprovedUsersProvider interface {
	List(ctx context.Context, tenantID string) ([]approved.UserInfo, error)
	Add(ctx context.Context, tenantID string, user approved.UserInfo) error
	Remove(ctx context.Context, tenantID string, id string) error
}

// DictionaryProvider provides access to dictionary management through the control plane service layer.
type DictionaryProvider interface {
	Dictionary
}

// RuleSetProvider provides access to the active rule set and allows runtime updates.
type RuleSetProvider interface {
	Get(ctx context.Context, workspaceID string) (rules.RuleSet, error)
	Update(ctx context.Context, workspaceID string, source string, rs rules.RuleSet) (rules.RuleSet, error)
}

// ControlPlaneAuthorizer checks workspace role permissions for control plane endpoints.
type ControlPlaneAuthorizer interface {
	Authorize(ctx context.Context, workspaceID string, userID string, access string) error
}

// NewServer creates a new web API server.
func NewServer(config Config) *Server {
	return &Server{Config: config}
}

// Run starts server and accepts requests checking for spam messages.
func (s *Server) Run(ctx context.Context) error {
	router := routegroup.New(http.NewServeMux())
	router.Use(rest.Recoverer(log.Default()))
	router.Use(SecurityHeaders)
	router.Use(NewAdminAuditLogger().Middleware)
	router.Use(SanitizeInputMiddleware)
	router.Use(s.requestMetadataMiddleware)
	router.Use(logger.New(logger.Log(log.Default()), logger.Prefix("[DEBUG]")).Handler)
	router.Use(rest.Throttle(1000))
	router.Use(rest.AppInfo("tg-spam", "redstone-md", s.Version), rest.Ping)
	router.Use(s.tenantRateLimitMiddleware())
	router.Use(s.tenantAuthzMiddleware())
	router.Use(rest.SizeLimit(1024 * 1024))

	if s.AuthPasswd != "" || s.AuthHash != "" {
		log.Printf("[INFO] basic auth enabled for webapi server")
		if s.AuthHash != "" {
			router.Use(rest.BasicAuthWithBcryptHashAndPrompt("tg-spam", s.AuthHash))
		} else {
			router.Use(rest.BasicAuthWithPrompt("tg-spam", s.AuthPasswd))
		}
	} else {
		log.Printf("[WARN] basic auth disabled, access to webapi is not protected")
	}

	router = s.routes(router) // setup routes

	srv := &http.Server{Addr: s.ListenAddr, Handler: router, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("[WARN] failed to shutdown webapi server: %v", err)
		} else {
			log.Printf("[INFO] webapi server stopped")
		}
	}()

	log.Printf("[INFO] start webapi server on %s", s.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to run server: %w", err)
	}
	return nil
}

func (s *Server) requestMetadataMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventID, correlationID := s.requestMetadata(r)
		ctx := observability.WithEventMetadata(r.Context(), eventID, correlationID)
		w.Header().Set(requestHeaderEventID, eventID)
		w.Header().Set(requestHeaderCorrelationID, correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requestMetadata(r *http.Request) (eventID, correlationID string) {
	seq := requestSeq.Add(1)
	instanceID := strings.TrimSpace(s.Settings.TenantID)
	if instanceID == "" {
		instanceID = "tg-spam"
	}

	eventID = strings.TrimSpace(r.Header.Get(requestHeaderEventID))
	if eventID == "" {
		eventID = fmt.Sprintf("web-%s-%d", instanceID, seq)
	}

	correlationID = strings.TrimSpace(r.Header.Get(requestHeaderCorrelationID))
	if correlationID == "" {
		correlationID = strings.TrimSpace(r.Header.Get(requestHeaderRequestID))
	}
	if correlationID == "" {
		correlationID = fmt.Sprintf("corr-web-%s-%d", instanceID, seq)
	}

	return eventID, correlationID
}

func (s *Server) authMiddleware(mw func(next http.Handler) http.Handler) func(next http.Handler) http.Handler {
	if s.AuthPasswd == "" {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return func(next http.Handler) http.Handler {
		return mw(next)
	}
}

func (s *Server) tenantAuthzMiddleware() func(next http.Handler) http.Handler {
	if s.TenantStatusProvider == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	mw := &TenantStatusMiddleware{Checker: s.TenantStatusProvider, TenantID: s.Settings.TenantID}
	return mw.Handler
}

func (s *Server) tenantRateLimitMiddleware() func(next http.Handler) http.Handler {
	if s.RateLimiter == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return s.RateLimiter.Middleware
}

func (s *Server) controlPlaneAuthMiddleware(next http.Handler) http.Handler {
	if s.ControlPlaneAuth == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := r.BasicAuth()
		if !ok || strings.TrimSpace(userID) == "" {
			_ = rest.EncodeJSON(w, http.StatusUnauthorized, rest.JSON{"error": "control plane authentication required"})
			return
		}

		access := "read"
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			access = "write"
		}
		if err := s.ControlPlaneAuth.Authorize(r.Context(), s.Settings.TenantID, userID, access); err != nil {
			_ = rest.EncodeJSON(w, http.StatusForbidden, rest.JSON{"error": "control plane access denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
