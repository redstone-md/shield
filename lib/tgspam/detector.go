package tgspam

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/plugin"
)

//go:generate moq --out mocks/sample_updater.go --pkg mocks --skip-ensure --with-resets . SampleUpdater
//go:generate moq --out mocks/http_client.go --pkg mocks --skip-ensure --with-resets . HTTPClient
//go:generate moq --out mocks/user_storage.go --pkg mocks --skip-ensure --with-resets . UserStorage
//go:generate moq --out mocks/lua_plugin_engine.go --pkg mocks --skip-ensure --with-resets . LuaPluginEngine

// Detector is a spam detector, thread-safe.
// It uses a set of checks to determine if a message is spam, and also keeps a list of approved users.
type Detector struct {
	Config
	classifier        classifier
	openaiChecker     *openAIChecker
	geminiChecker     *geminiChecker
	duplicateDetector *duplicateDetector
	metaChecks        []MetaCheck
	luaChecks         []plugin.Check // separate field for Lua plugin checks
	tokenizedSpam     []map[string]int
	approvedUsers     map[string]approved.UserInfo
	stopWords         []string
	excludedTokens    map[string]struct{}
	luaEngine         LuaPluginEngine

	spamSamplesUpd SampleUpdater
	hamSamplesUpd  SampleUpdater
	userStorage    UserStorage

	// history of recent messages to keep in memory
	// can be passed to checkers supporting history
	hamHistory  *spamcheck.LastRequests
	spamHistory *spamcheck.LastRequests
	llmHistory  *spamcheck.LastRequests
	userHistory map[string]*spamcheck.LastRequests

	lock sync.RWMutex
}

// LLMConsensusMode controls how eligible LLM checks flip the base decision.
type LLMConsensusMode string

// LLMMode controls which base detector outcomes are sent to LLM checks.
type LLMMode string

const (
	// LLMConsensusAny flips the base decision if any eligible LLM agrees.
	LLMConsensusAny LLMConsensusMode = "any"
	// LLMConsensusAll flips the base decision only if all eligible LLMs agree.
	LLMConsensusAll LLMConsensusMode = "all"
)

const (
	// LLMModeMissed checks messages the base detector allowed through.
	LLMModeMissed LLMMode = "missed"
	// LLMModeFlagged checks messages the base detector already flagged.
	LLMModeFlagged LLMMode = "flagged"
	// LLMModeAlways checks both allowed and flagged messages.
	LLMModeAlways LLMMode = "always"
)

const (
	llmChatContextSize = 5
	llmUserContextSize = 5
)

// detectorLLMCheck describes how a single LLM provider participates in Detector.Check.
type detectorLLMCheck struct {
	name    string // provider name used in logs
	enabled bool   // whether this provider is configured
	// whether short messages should still be sent to the provider
	checkShortMessages bool
	// mode controls whether this provider checks missed, flagged, or all messages.
	mode  LLMMode
	check func(context.Context, string, llmContext) (bool, spamcheck.Response) // provider check function
}

type detectorLLMResult struct {
	details spamcheck.Response
	flip    bool
}

// Config is a set of parameters for Detector.
type Config struct {
	SimilarityThreshold float64          // threshold for spam similarity, 0.0 - 1.0
	MinMsgLen           int              // minimum message length to check
	MaxAllowedEmoji     int              // maximum number of emojis allowed in a message
	CasAPI              string           // CAS API URL
	CasUserAgent        string           // CAS API User-Agent header value, set only if non-empty
	FirstMessageOnly    bool             // if true, only the first message from a user is checked
	FirstMessagesCount  int              // number of first messages to check for spam
	HTTPClient          HTTPClient       // http client to use for requests
	MinSpamProbability  float64          // minimum spam probability to consider a message spam with classifier, if 0 - ignored
	OpenAIVeto          bool             // if true, openai vetos spam, otherwise vetos ham
	OpenAIHistorySize   int              // history size for openai
	GeminiVeto          bool             // if true, gemini vetos spam, otherwise vetos ham
	GeminiHistorySize   int              // history size for gemini
	LLMMode             LLMMode          // which base detector outcomes are sent to LLM checks
	LLMConsensus        LLMConsensusMode // how eligible LLM checks flip the base decision
	LLMRequestTimeout   time.Duration    // timeout for individual LLM requests, if not set - 30s default
	MultiLangWords      int              // if true, check for number of multi-lingual words
	StorageTimeout      time.Duration    // timeout for storage operations, if not set - no timeout

	LuaPlugins struct {
		Enabled        bool     // if true, enable Lua plugins
		PluginsDir     string   // directory with Lua plugins
		EnabledPlugins []string // list of enabled plugins (by name, without .lua extension)
		DynamicReload  bool     // if true, enable dynamic reloading of Lua plugins when files change
	}

	AbnormalSpacing struct {
		Enabled                 bool    // if true, enable check for abnormal spacing
		MinWordsCount           int     // the minimum number of words in the message to be considered
		ShortWordLen            int     // the length of the word to be considered short (in rune characters)
		ShortWordRatioThreshold float64 // the ratio of short words to all words in the message
		SpaceRatioThreshold     float64 // the ratio of spaces to all characters in the message
	}

	DuplicateDetection struct {
		Threshold int           // number of duplicate messages to trigger spam (0=disabled)
		Window    time.Duration // time window for duplicate detection
	}

	HistorySize int // history of recent messages to keep in memory

	ScoringThreshold float64 // threshold for scoring engine aggregation, 0 = disabled (use boolean OR)
}

// SampleUpdater is an interface for updating spam/ham samples on the fly.
type SampleUpdater interface {
	Append(msg string) error        // append a message to the samples storage
	Remove(msg string) error        // remove a message from the samples storage
	Reader() (io.ReadCloser, error) // return a reader for the samples storage
}

// UserStorage is an interface for approved users storage.
type UserStorage interface {
	Read(ctx context.Context) ([]approved.UserInfo, error) // read approved users from storage
	Write(ctx context.Context, au approved.UserInfo) error // write approved user to storage
	Delete(ctx context.Context, id string) error           // delete approved user from storage
}

// HTTPClient is an interface for http client, satisfied by http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// LuaPluginEngine defines an interface for the Lua plugin system
type LuaPluginEngine interface {
	LoadScript(path string) error               // loads a single Lua script
	ReloadScript(path string) error             // reloads a single Lua script
	LoadDirectory(dir string) error             // loads all Lua scripts from a directory
	GetCheck(name string) (plugin.Check, error) // returns a specific named plugin check
	GetAllChecks() map[string]plugin.Check      // returns all loaded plugin checks
	Close()                                     // cleans up resources
}

// LoadResult is a result of loading samples.
type LoadResult struct {
	ExcludedTokens int // number of excluded tokens
	SpamSamples    int // number of spam samples
	HamSamples     int // number of ham samples
	StopWords      int // number of stop words (phrases)
}

// NewDetector makes a new Detector with the given config.
func NewDetector(p Config) *Detector {
	res := &Detector{
		Config:            p,
		classifier:        newClassifier(),
		approvedUsers:     make(map[string]approved.UserInfo),
		tokenizedSpam:     []map[string]int{},
		metaChecks:        []MetaCheck{},
		luaChecks:         []plugin.Check{},
		hamHistory:        spamcheck.NewLastRequests(p.HistorySize),
		spamHistory:       spamcheck.NewLastRequests(p.HistorySize),
		llmHistory:        spamcheck.NewLastRequests(max(p.HistorySize, llmChatContextSize)),
		userHistory:       make(map[string]*spamcheck.LastRequests),
		duplicateDetector: newDuplicateDetector(p.DuplicateDetection.Threshold, p.DuplicateDetection.Window),
		luaEngine:         nil, // will be set with WithLuaEngine if needed
	}
	res.LLMConsensus = res.normalizeLLMConsensusMode(p.LLMConsensus)
	// if FirstMessagesCount is set, FirstMessageOnly enforced to true.
	// this is to avoid confusion when FirstMessagesCount is set but FirstMessageOnly is false.
	// the reason for the redundant FirstMessageOnly flag is to avoid breaking api compatibility.
	if p.FirstMessagesCount > 0 {
		res.FirstMessageOnly = true
	}
	if p.FirstMessageOnly && p.FirstMessagesCount == 0 {
		res.FirstMessagesCount = 1 // default value for FirstMessagesCount if FirstMessageOnly is set
	}
	return res
}

// Check checks if a given message is spam. Returns true if spam and also returns a list of check results.
func (d *Detector) Check(req spamcheck.Request) (spam bool, cr []spamcheck.Response) {

	isSpamDetected := func(cr []spamcheck.Response) bool {
		for _, r := range cr {
			if r.Spam {
				return true
			}
		}
		return false
	}

	cleanMsg := d.cleanText(req.Msg)
	d.lock.Lock()
	defer d.lock.Unlock()

	// check for duplicate messages FIRST - behavioral check that applies to all users
	if d.duplicateDetector != nil {
		cr = append(cr, d.duplicateDetector.check(req))
	}

	// approved user don't need content analysis checks, but only skip if no spam detected by behavioral checks
	if req.UserID != "" && d.FirstMessageOnly && !isSpamDetected(cr) && d.approvedUsers[req.UserID].Count >= d.FirstMessagesCount {
		// include previous check results (e.g., duplicate check) in the response
		return false, append(cr, spamcheck.Response{Name: "pre-approved", Spam: false, Details: "user already approved"})
	}

	// all the remaining checks are performed sequentially, so we can collect all the results

	// check for stop words if any stop words are loaded
	if len(d.stopWords) > 0 {
		cr = append(cr, d.isStopWord(cleanMsg, req))
	}

	// check for emojis if max allowed emojis is set
	if d.MaxAllowedEmoji >= 0 {
		cr = append(cr, d.isManyEmojis(req.Msg))
	}

	// check for spam with meta-checks
	for _, mc := range d.metaChecks {
		cr = append(cr, mc(req))
	}

	// check for spam with Lua plugin checks
	for _, lc := range d.luaChecks {
		cr = append(cr, lc(req))
	}

	// check for spam with CAS API if CAS API URL is set
	if d.CasAPI != "" {
		cr = append(cr, d.isCasSpam(req.UserID))
	}

	if d.MultiLangWords > 0 {
		cr = append(cr, d.isMultiLang(req.Msg))
	}

	if d.AbnormalSpacing.Enabled {
		cr = append(cr, d.isAbnormalSpacing(req.Msg))
	}

	// check for message length exceed the minimum size, if min message length is set.
	// the check is done after first simple checks, because stop words and emojis can be triggered by short messages as well.
	isShortMessage := false
	if len([]rune(req.Msg)) < d.MinMsgLen {
		isShortMessage = true
		cr = append(cr, spamcheck.Response{Name: "message length", Spam: false, Details: "too short"})
		// only return early if:
		// 1. we already detected spam from simple checks above, OR
		// 2. no LLM checker is configured for short messages, OR
		// 3. LLM checkers are configured but LLMs won't run (FirstMessageOnly/FirstMessagesCount not set)
		openaiChecksShort := d.openaiChecker != nil && d.openaiChecker.params.CheckShortMessagesWithOpenAI
		geminiChecksShort := d.geminiChecker != nil && d.geminiChecker.params.CheckShortMessages
		llmEligible := d.hasLLMEnabled()
		forceLLM := req.ForceLLM && llmEligible
		if isSpamDetected(cr) || !llmEligible || (!forceLLM && !openaiChecksShort && !geminiChecksShort) {
			if isSpamDetected(cr) {
				d.spamHistory.Push(req)
				return true, cr // spam from the checks above
			}
			// don't add short messages to hamHistory as they haven't been properly checked
			return false, cr
		}
		// if we get here, we have a short message but an eligible LLM should still check it
	}

	// check for spam similarity if a similarity threshold is set and spam samples are loaded
	// skip for short messages as similarity doesn't work well on short text
	if !isShortMessage && d.SimilarityThreshold > 0 && len(d.tokenizedSpam) > 0 {
		cr = append(cr, d.isSpamSimilarityHigh(cleanMsg))
	}

	// check for spam with classifier if classifier is loaded
	// skip for short messages as classifier doesn't work well on short text
	classifierReady := d.classifier.nAllDocument > 0 &&
		d.classifier.nDocumentByClass["ham"] > 0 && d.classifier.nDocumentByClass["spam"] > 0
	if !isShortMessage && classifierReady {
		cr = append(cr, d.isSpamClassified(cleanMsg))
	}

	baseSpam := d.scoreSignals(cr, isSpamDetected)
	spamDetected := baseSpam

	// we hit eligible LLMs in three cases:
	// - short message with short-message checking enabled (ignores veto mode since there's no decision to veto)
	// - all checks passed (ham) and veto is false - improves false negative rate
	// - checks failed (spam) and veto is true - improves false positive rate
	if d.hasLLMEnabled() {
		llmResults := make([]detectorLLMResult, 0, 2)
		llmChecks := []detectorLLMCheck{
			{
				name:               "openai",
				enabled:            d.openaiChecker != nil,
				checkShortMessages: d.openaiChecker != nil && d.openaiChecker.params.CheckShortMessagesWithOpenAI,
				mode:               d.normalizeLLMMode(d.LLMMode, d.OpenAIVeto),
				check: func(ctx context.Context, msg string, history llmContext) (bool, spamcheck.Response) {
					return d.openaiChecker.check(ctx, msg, history)
				},
			},
			{
				name:               "gemini",
				enabled:            d.geminiChecker != nil,
				checkShortMessages: d.geminiChecker != nil && d.geminiChecker.params.CheckShortMessages,
				mode:               d.normalizeLLMMode(d.LLMMode, d.GeminiVeto),
				check: func(ctx context.Context, msg string, history llmContext) (bool, spamcheck.Response) {
					return d.geminiChecker.check(ctx, msg, history)
				},
			},
		}

		for _, llmCheck := range llmChecks {
			if res, ok := d.collectLLMCheck(req, cleanMsg, cr, baseSpam, isShortMessage, llmCheck); ok {
				cr = append(cr, res.details)
				llmResults = append(llmResults, res)
			}
		}

		spamDetected = d.applyLLMConsensus(baseSpam, llmResults, d.LLMConsensus)
	}

	// CAS is a curated global ban list, so an LLM ham verdict must not veto a CAS hit
	if !spamDetected && casFlaggedSpam(cr) {
		spamDetected = true
	}

	if spamDetected {
		d.spamHistory.Push(req)
		return true, cr
	}

	// update approved users only if it's not paranoid mode and not a check-only request
	// and only if the message was not too short (to ensure we have meaningful content)
	if (d.FirstMessageOnly || d.FirstMessagesCount > 0) && !req.CheckOnly && !isShortMessage {
		ctx, cancel := d.ctxWithStoreTimeout()
		defer cancel()
		au := approved.UserInfo{
			Count:     d.approvedUsers[req.UserID].Count + 1,
			UserID:    req.UserID,
			UserName:  req.UserName,
			Timestamp: time.Now(),
		}
		d.approvedUsers[req.UserID] = au // update approved users status in memory
		if d.userStorage != nil {
			// update approved users status in storage
			_ = d.userStorage.Write(ctx, au) // ignore error, failed to write to storage is not critical here
		}
	}
	d.addToLLMHistory(req)
	d.hamHistory.Push(req)
	return false, cr
}

func (d *Detector) normalizeLLMConsensusMode(mode LLMConsensusMode) LLMConsensusMode {
	if mode == LLMConsensusAll {
		return mode
	}
	return LLMConsensusAny
}

func (d *Detector) normalizeLLMMode(mode LLMMode, veto bool) LLMMode {
	switch mode {
	case LLMModeMissed, LLMModeFlagged, LLMModeAlways:
		return mode
	}
	if veto {
		return LLMModeFlagged
	}
	return LLMModeMissed
}

func (d *Detector) hasLLMEnabled() bool {
	return d.openaiChecker != nil || d.geminiChecker != nil
}

func (d *Detector) shouldApplyLLMCheck(baseSpam, isShortMessage, forceLLM bool, cfg detectorLLMCheck) bool {
	if forceLLM {
		return true
	}
	if isShortMessage {
		return cfg.checkShortMessages
	}
	switch cfg.mode {
	case LLMModeAlways:
		return true
	case LLMModeFlagged:
		return baseSpam
	default:
		return !baseSpam
	}
}

func (d *Detector) collectLLMCheck(req spamcheck.Request, cleanMsg string, cr []spamcheck.Response,
	baseSpam bool, isShortMessage bool, cfg detectorLLMCheck,
) (detectorLLMResult, bool) {
	if !cfg.enabled || cfg.check == nil {
		return detectorLLMResult{}, false
	}

	if !d.shouldApplyLLMCheck(baseSpam, isShortMessage, req.ForceLLM, cfg) {
		return detectorLLMResult{}, false
	}

	if shouldSkipTextLLM(cleanMsg) {
		log.Printf("[DEBUG] %s skipped: empty message has no text content for LLM", cfg.name)
		return detectorLLMResult{}, false
	}

	hist := d.llmContextForRequest(req)

	ctx, cancel := d.ctxWithLLMTimeout()
	defer cancel()

	llmMsg := cleanMsg
	if req.LLMMessage != "" {
		llmMsg = req.LLMMessage
	}
	spam, details := cfg.check(ctx, llmMsg, hist)
	if baseSpam && details.Error != nil {
		log.Printf("[WARN] %s error: %v", cfg.name, details.Error)
	}

	log.Printf("[DEBUG] %s result: {%s}", cfg.name, details.String())

	if cfg.mode == LLMModeFlagged && !spam && details.Error == nil {
		allChecks := append(append(make([]spamcheck.Response, 0, len(cr)+1), cr...), details)
		log.Printf("[DEBUG] %s vetoed ham message: %q, checks: %s", cfg.name, req.Msg, spamcheck.ChecksToString(allChecks))
	}

	flip := false
	if details.Error == nil {
		flip = (!baseSpam && spam) || (baseSpam && !spam)
	}

	return detectorLLMResult{details: details, flip: flip}, true
}

func shouldSkipTextLLM(msg string) bool {
	return strings.TrimSpace(msg) == ""
}

// casFlaggedSpam reports whether the CAS check flagged the message as spam.
func casFlaggedSpam(cr []spamcheck.Response) bool {
	for _, r := range cr {
		if r.Name == "cas" && r.Spam {
			return true
		}
	}
	return false
}

func (d *Detector) llmContextForRequest(req spamcheck.Request) llmContext {
	ctx := llmContext{
		RequestContext:     req.LLMContext,
		RecentChatMessages: d.llmHistory.Last(llmChatContextSize),
	}
	return ctx
}

func (d *Detector) addToLLMHistory(req spamcheck.Request) {
	if req.CheckOnly || req.Msg == "" {
		return
	}

	d.llmHistory.Push(req)
	if req.UserID == "" {
		return
	}

	h, ok := d.userHistory[req.UserID]
	if !ok {
		h = spamcheck.NewLastRequests(llmUserContextSize)
		d.userHistory[req.UserID] = h
	}
	h.Push(req)
}

func (d *Detector) applyLLMConsensus(baseSpam bool, results []detectorLLMResult, mode LLMConsensusMode) bool {
	if len(results) == 0 {
		return baseSpam
	}

	switch d.normalizeLLMConsensusMode(mode) {
	case LLMConsensusAll:
		for _, result := range results {
			if !result.flip {
				return baseSpam
			}
		}
		return !baseSpam
	default:
		for _, result := range results {
			if result.flip {
				return !baseSpam
			}
		}
		return baseSpam
	}
}

// Reset resets spam samples/classifier, excluded tokens, stop words and approved users.
func (d *Detector) Reset() {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.tokenizedSpam = []map[string]int{}
	d.excludedTokens = map[string]struct{}{}
	d.classifier.reset()
	d.approvedUsers = make(map[string]approved.UserInfo)
	d.stopWords = []string{}
	d.llmHistory = spamcheck.NewLastRequests(max(d.HistorySize, llmChatContextSize))
	d.userHistory = make(map[string]*spamcheck.LastRequests)

	// close the Lua engine and reset Lua checks if it exists
	if d.luaEngine != nil {
		d.luaEngine.Close()
		d.luaEngine = nil
		d.luaChecks = nil
	}
}
