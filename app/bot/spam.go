package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/approved"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/redstone-md/shield/lib/tgspam"
)

//go:generate go run github.com/matryer/moq@latest --out mocks/message_checker.go --pkg mocks --skip-ensure --with-resets . MessageChecker
//go:generate go run github.com/matryer/moq@latest --out mocks/sample_loader.go --pkg mocks --skip-ensure --with-resets . SampleLoader
//go:generate go run github.com/matryer/moq@latest --out mocks/sample_updater.go --pkg mocks --skip-ensure --with-resets . SampleUpdater
//go:generate go run github.com/matryer/moq@latest --out mocks/approved_users.go --pkg mocks --skip-ensure --with-resets . ApprovedUsers
//go:generate go run github.com/matryer/moq@latest --out mocks/samples.go --pkg mocks --skip-ensure --with-resets . SamplesStore
//go:generate go run github.com/matryer/moq@latest --out mocks/dictionary.go --pkg mocks --skip-ensure --with-resets . DictStore

// SpamFilter bot checks if a user is a spammer using lib.Detector
// Reloads spam samples, stop words and excluded tokens on file change.
type SpamFilter struct {
	checker  MessageChecker
	loader   SampleLoader
	updater  SampleUpdater
	approved ApprovedUsers
	params   SpamConfig
}

// SpamConfig is a full set of parameters for spam bot
type SpamConfig struct {
	SamplesStore SamplesStore // storage for spam samples
	DictStore    DictStore    // storage for stop words and excluded tokens

	SpamMsg    string
	SpamDryMsg string
	TenantID   string
	Dry        bool
}

// Detector is a full detector interface used by production wiring.
type Detector interface {
	MessageChecker
	SampleLoader
	SampleUpdater
	ApprovedUsers
}

// MessageChecker checks a message for spam.
type MessageChecker interface {
	Check(request spamcheck.Request) (spam bool, cr []spamcheck.Response)
}

// SampleLoader reloads detector samples and stop words.
type SampleLoader interface {
	LoadSamples(exclReader io.Reader, spamReaders, hamReaders []io.Reader) (tgspam.LoadResult, error)
	LoadStopWords(readers ...io.Reader) (tgspam.LoadResult, error)
}

// SampleUpdater updates detector training samples.
type SampleUpdater interface {
	UpdateSpam(msg string) error
	UpdateHam(msg string) error
	RemoveHam(msg string) error
	RemoveSpam(msg string) error
}

// ApprovedUsers manages detector approved users.
type ApprovedUsers interface {
	AddApprovedUser(user approved.UserInfo) error
	RemoveApprovedUser(id string) error
	ApprovedUsers() (res []approved.UserInfo)
	IsApprovedUser(userID string) bool
}

// SamplesStore is a storage for spam samples
type SamplesStore interface {
	Read(ctx context.Context, t storage.SampleType, o storage.SampleOrigin) ([]string, error)
	Reader(ctx context.Context, t storage.SampleType, o storage.SampleOrigin) (io.ReadCloser, error)
	Stats(ctx context.Context) (*storage.SamplesStats, error)
}

// DictStore is a storage for dictionaries, i.e. stop words and ignored words
type DictStore interface {
	Reader(ctx context.Context, t storage.DictionaryType) (io.ReadCloser, error)
}

// NewSpamFilter creates new spam filter.
func NewSpamFilter(detector MessageChecker, params SpamConfig) *SpamFilter {
	res := NewSpamFilterWithRoles(detector, nil, nil, nil, params)
	if loader, ok := detector.(SampleLoader); ok {
		res.loader = loader
	}
	if updater, ok := detector.(SampleUpdater); ok {
		res.updater = updater
	}
	if approvedUsers, ok := detector.(ApprovedUsers); ok {
		res.approved = approvedUsers
	}
	return res
}

// NewSpamFilterWithRoles creates a spam filter from role-specific detector dependencies.
func NewSpamFilterWithRoles(
	checker MessageChecker, loader SampleLoader, updater SampleUpdater,
	approvedUsers ApprovedUsers, params SpamConfig,
) *SpamFilter {
	return &SpamFilter{checker: checker, loader: loader, updater: updater, approved: approvedUsers, params: params}
}

// OnMessage checks if user already approved and if not checks if user is a spammer
func (s *SpamFilter) OnMessage(msg Message, checkOnly bool) (response Response) {
	return s.OnMessageWithContext(context.Background(), msg, checkOnly)
}

// OnMessageWithContext checks if user already approved and if not checks if user is a spammer.
func (s *SpamFilter) OnMessageWithContext(ctx context.Context, msg Message, checkOnly bool) (response Response) {
	if msg.From.ID == 0 { // don't check system messages
		return Response{}
	}
	displayUsername := DisplayName(msg)

	msgText := msg.Text

	// use channel identity for spam check when message is from a channel,
	// so that approved/banned status is tracked per-channel, not for the shared Channel_Bot user
	checkUserID := msg.From.ID
	checkUserName := msg.From.Username
	firstName, lastName, isPremium := msg.From.FirstName, msg.From.LastName, msg.From.IsPremium
	if msg.SenderChat.ID != 0 {
		checkUserID = msg.SenderChat.ID
		checkUserName = msg.SenderChat.UserName
		firstName, lastName, isPremium = "", "", false // channels don't have personal user fields
	}

	spamReq := spamcheck.Request{Msg: msgText, CheckOnly: checkOnly,
		UserID: strconv.FormatInt(checkUserID, 10), UserName: checkUserName,
		FirstName: firstName, LastName: lastName, IsPremium: isPremium,
		ForceLLM: msg.ForceLLM, LLMContext: msg.LLMContext}
	if msg.Image != nil {
		spamReq.Meta.Images = 1
	}
	if msg.WithVideo || msg.WithVideoNote {
		spamReq.Meta.HasVideo = true
	}
	if msg.WithAudio {
		spamReq.Meta.HasAudio = true
	}
	if msg.WithForward {
		spamReq.Meta.HasForward = true
	}
	if msg.WithKeyboard {
		spamReq.Meta.HasKeyboard = true
	}
	if msg.WithContact {
		spamReq.Meta.HasContact = true
	}
	if msg.WithGiveaway {
		spamReq.Meta.HasGiveaway = true
	}
	if msg.WithSticker {
		spamReq.Meta.HasSticker = true
	}
	spamReq.Meta.MessageID = msg.ID

	// count mentions and links from entities (both regular and caption entities)
	// links are counted from entities only - telegram provides url/text_link entities for all links
	if msg.Entities != nil {
		for _, entity := range *msg.Entities {
			switch entity.Type {
			case "mention", "text_mention":
				spamReq.Meta.Mentions++
			case "url", "text_link":
				spamReq.Meta.Links++
			}
		}
	}
	if msg.Image != nil && msg.Image.Entities != nil {
		for _, entity := range *msg.Image.Entities {
			switch entity.Type {
			case "mention", "text_mention":
				spamReq.Meta.Mentions++
			case "url", "text_link":
				spamReq.Meta.Links++
			}
		}
	}
	isSpam, checkResults := s.checker.Check(spamReq)
	crs := make([]string, 0, len(checkResults))
	for _, cr := range checkResults {
		crs = append(crs, fmt.Sprintf("{name: %s, spam: %v, details: %s}", cr.Name, cr.Spam, cr.Details))
	}
	checkResultStr := strings.Join(crs, ", ")
	if isSpam {
		observability.Logf(ctx, "[INFO] user %s detected as spammer: %s, %q", displayUsername, checkResultStr, msgText)
		msgPrefix := s.params.SpamMsg
		if s.params.Dry {
			msgPrefix = s.params.SpamDryMsg
		}
		msgPrefix = spamVerdictText(checkResults, msgPrefix)
		spamRespMsg := fmt.Sprintf("%s: %q (%d)", msgPrefix, displayUsername, msg.From.ID)
		return Response{Text: spamRespMsg, Send: true, ReplyTo: msg.ID, BanInterval: PermanentBanDuration, CheckResults: checkResults,
			DeleteReplyTo: true, User: User{Username: msg.From.Username, ID: msg.From.ID, DisplayName: msg.From.DisplayName},
			ChannelID: msg.SenderChat.ID,
		}
	}
	observability.Logf(ctx, "[DEBUG] user %s is not a spammer, %s", displayUsername, checkResultStr)
	return Response{CheckResults: checkResults} // not a spam
}

func spamVerdictText(results []spamcheck.Response, fallback string) string {
	for _, result := range results {
		if !result.Spam {
			continue
		}
		if result.Name != "openai" && result.Name != "gemini" {
			continue
		}
		reason := strings.TrimSpace(result.Details)
		if idx := strings.LastIndex(reason, ", confidence:"); idx >= 0 {
			reason = reason[:idx]
		}
		reason = strings.TrimSuffix(strings.TrimSpace(reason), ".")
		if reason != "" {
			return reason
		}
	}
	return fallback
}

// UpdateSpam appends a message to the spam samples file and updates the classifier
func (s *SpamFilter) UpdateSpam(msg string) error {
	cleanMsg := strings.ReplaceAll(msg, "\n", " ")
	log.Printf("[DEBUG] update spam samples with %q", cleanMsg)
	if s.updater == nil {
		return fmt.Errorf("sample updater not configured")
	}
	if err := s.updater.UpdateSpam(cleanMsg); err != nil {
		return fmt.Errorf("can't update spam samples: %w", err)
	}
	log.Printf("[INFO] updated spam samples with %q", cleanMsg)
	return nil
}

// UpdateHam appends a message to the ham samples file and updates the classifier
func (s *SpamFilter) UpdateHam(msg string) error {
	cleanMsg := strings.ReplaceAll(msg, "\n", " ")
	log.Printf("[DEBUG] update ham samples with %q", cleanMsg)
	if s.updater == nil {
		return fmt.Errorf("sample updater not configured")
	}
	if err := s.updater.UpdateHam(cleanMsg); err != nil {
		return fmt.Errorf("can't update ham samples: %w", err)
	}
	log.Printf("[INFO] updated ham samples with %q", cleanMsg)
	return nil
}

// IsApprovedUser checks if user is in the list of approved users
func (s *SpamFilter) IsApprovedUser(userID int64) bool {
	return s.approved != nil && s.approved.IsApprovedUser(fmt.Sprintf("%d", userID))
}

// AddApprovedUser adds users to the list of approved users, to both the detector and the storage
func (s *SpamFilter) AddApprovedUser(id int64, name string) error {
	log.Printf("[INFO] add aproved user: id:%d, name:%q", id, name)
	if s.approved == nil {
		return fmt.Errorf("approved users store not configured")
	}
	if err := s.approved.AddApprovedUser(approved.UserInfo{UserID: fmt.Sprintf("%d", id), UserName: name}); err != nil {
		return fmt.Errorf("failed to write approved user to storage: %w", err)
	}
	return nil
}

// RemoveApprovedUser removes users from the list of approved users in both the detector and the storage
func (s *SpamFilter) RemoveApprovedUser(id int64) error {
	log.Printf("[INFO] remove approved user: %d", id)
	if s.approved == nil {
		return fmt.Errorf("approved users store not configured")
	}
	if err := s.approved.RemoveApprovedUser(fmt.Sprintf("%d", id)); err != nil {
		return fmt.Errorf("failed to delete approved user from storage: %w", err)
	}
	return nil
}

// ReloadSamples reloads samples and stop-words
func (s *SpamFilter) ReloadSamples(ctx context.Context) (err error) {
	log.Printf("[DEBUG] reloading samples")
	if s.loader == nil {
		return fmt.Errorf("sample loader not configured")
	}

	var exclReader, spamReader, hamReader, stopWordsReader, spamDynamicReader, hamDynamicReader io.ReadCloser

	// check mandatory data presence
	st, err := s.params.SamplesStore.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get samples store stats: %w", err)
	}
	if st.PresetSpam == 0 || st.PresetHam == 0 {
		return fmt.Errorf("no pesistent spam or ham samples found in the store")
	}

	if spamReader, err = s.params.SamplesStore.Reader(ctx, storage.SampleTypeSpam, storage.SampleOriginPreset); err != nil {
		return fmt.Errorf("failed to get persistent spam samples: %w", err)
	}
	defer spamReader.Close()

	if hamReader, err = s.params.SamplesStore.Reader(ctx, storage.SampleTypeHam, storage.SampleOriginPreset); err != nil {
		return fmt.Errorf("failed to get persistent ham samples: %w", err)
	}
	defer hamReader.Close()

	if spamDynamicReader, err = s.params.SamplesStore.Reader(ctx, storage.SampleTypeSpam, storage.SampleOriginUser); err != nil {
		return fmt.Errorf("failed to get dynamic spam samples: %w", err)
	}
	defer spamDynamicReader.Close()

	if hamDynamicReader, err = s.params.SamplesStore.Reader(ctx, storage.SampleTypeHam, storage.SampleOriginUser); err != nil {
		return fmt.Errorf("failed to get dynamic ham samples: %w", err)
	}
	defer hamDynamicReader.Close()

	// stop-words are optional
	if stopWordsReader, err = s.params.DictStore.Reader(ctx, storage.DictionaryTypeStopPhrase); err != nil {
		return fmt.Errorf("failed to get stop words: %w", err)
	}
	defer stopWordsReader.Close()

	// excluded tokens are optional
	if exclReader, err = s.params.DictStore.Reader(ctx, storage.DictionaryTypeIgnoredWord); err != nil {
		return fmt.Errorf("failed to get excluded tokens: %w", err)
	}
	defer exclReader.Close()

	// reload samples and stop-words. note: we don't need reset as LoadSamples and LoadStopWords clear the state first
	lr, err := s.loader.LoadSamples(exclReader, []io.Reader{spamReader, spamDynamicReader},
		[]io.Reader{hamReader, hamDynamicReader})
	if err != nil {
		return fmt.Errorf("failed to reload samples: %w", err)
	}

	ls, err := s.loader.LoadStopWords(stopWordsReader)
	if err != nil {
		return fmt.Errorf("failed to reload stop words: %w", err)
	}

	log.Printf("[INFO] loaded samples - spam: %d, ham: %d, excluded tokens: %d, stop-words: %d",
		lr.SpamSamples, lr.HamSamples, lr.ExcludedTokens, ls.StopWords)

	return nil
}

// DynamicSamples returns dynamic spam and ham samples. both are optional
func (s *SpamFilter) DynamicSamples(ctx context.Context) (spam, ham []string, err error) {
	errs := new(multierror.Error)

	if spam, err = s.params.SamplesStore.Read(ctx, storage.SampleTypeSpam, storage.SampleOriginUser); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to read dynamic spam samples: %w", err))
	}

	if ham, err = s.params.SamplesStore.Read(ctx, storage.SampleTypeHam, storage.SampleOriginUser); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to read dynamic ham samples: %w", err))
	}

	if err := errs.ErrorOrNil(); err != nil {
		return spam, ham, fmt.Errorf("failed to read dynamic samples: %w", err)
	}
	return spam, ham, nil
}

// RemoveDynamicSpamSample removes a sample from the spam dynamic samples file and reloads samples after this
func (s *SpamFilter) RemoveDynamicSpamSample(sample string) error {
	cleanMsg := strings.ReplaceAll(sample, "\n", " ")
	log.Printf("[INFO] remove dynamic spam sample: %q", sample)
	if s.updater == nil {
		return fmt.Errorf("sample updater not configured")
	}
	if err := s.updater.RemoveSpam(cleanMsg); err != nil {
		return fmt.Errorf("can't remove spam sample %q: %w", sample, err)
	}
	return nil
}

// RemoveDynamicHamSample removes a sample from the ham dynamic samples file and reloads samples after this
func (s *SpamFilter) RemoveDynamicHamSample(sample string) error {
	cleanMsg := strings.ReplaceAll(sample, "\n", " ")
	log.Printf("[INFO] remove dynamic ham sample: %q", sample)
	if s.updater == nil {
		return fmt.Errorf("sample updater not configured")
	}
	if err := s.updater.RemoveHam(cleanMsg); err != nil {
		return fmt.Errorf("can't remove hma sample %q: %w", sample, err)
	}
	return nil
}

// ApplyRuleSet updates the spam filter's runtime config from a new rule set.
// It updates the dry-run flag and propagates detector-level config changes.
func (s *SpamFilter) ApplyRuleSet(rs rules.RuleSet) {
	s.params.Dry = rs.Moderation.DryRun
	log.Printf("[INFO] spam filter config updated: dry=%v", rs.Moderation.DryRun)
}
