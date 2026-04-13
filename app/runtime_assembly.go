package main

import (
	"context"
	"fmt"
	"io"
	"log"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/app/webapi"
	"github.com/umputun/tg-spam/lib/tgspam"
)

type runtimeAssembly struct {
	DataDB              *engine.SQL
	SpamBot             *bot.SpamFilter
	SpamLogger          events.SpamLogger
	LoggerWriter        io.Closer
	Locator             *storage.Locator
	RuleSets            *storage.RuleSets
	IncomingEventsStore *storage.IncomingEvents
	ReportsStore        *storage.Reports
	DetectedSpamStore   *storage.DetectedSpam
	TelegramListener    *events.TelegramListener
	Web                 webRuntimeAssembly
}

type webRuntimeAssembly struct {
	Detector      webapi.Detector
	SpamFilter    webapi.SpamFilter
	Locator       webapi.Locator
	StorageEngine webapi.StorageEngine
	BotUsername   string
}

func assembleRuntime(ctx context.Context, opts options, detector *tgspam.Detector) (*runtimeAssembly, error) {
	dataDB, err := makeDB(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("can't make db, %w", err)
	}

	spamBot, err := makeSpamBot(ctx, opts, dataDB, detector)
	if err != nil {
		return nil, fmt.Errorf("can't make spam bot, %w", err)
	}

	loggerWr, err := makeSpamLogWriter(opts)
	if err != nil {
		return nil, fmt.Errorf("can't make spam log writer, %w", err)
	}
	spamLogger, err := makeSpamLogger(ctx, opts.InstanceID, loggerWr, dataDB)
	if err != nil {
		_ = loggerWr.Close()
		return nil, fmt.Errorf("can't make spam logger, %w", err)
	}

	if opts.Convert == "only" {
		log.Print("[WARN] convert only mode, converting text samples and exit")
		return &runtimeAssembly{
			DataDB:       dataDB,
			SpamBot:      spamBot,
			SpamLogger:   spamLogger,
			LoggerWriter: loggerWr,
		}, nil
	}

	count, approvedUsersStore, err := makeApprovedUsersStore(ctx, dataDB, detector)
	if err != nil {
		return nil, err
	}
	_ = approvedUsersStore
	log.Printf("[DEBUG] approved users loaded: %d", count)

	locator, err := storage.NewLocator(ctx, opts.HistoryDuration, opts.HistoryMinSize, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make locator, %w", err)
	}

	ruleSets, err := storage.NewRuleSets(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make rule sets store, %w", err)
	}
	if _, err = ruleSets.EnsureBootstrap(ctx, bootstrapRuleSet(opts)); err != nil {
		return nil, fmt.Errorf("can't bootstrap rule set, %w", err)
	}

	incomingEventsStore, err := storage.NewIncomingEvents(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make incoming events store, %w", err)
	}

	var reportsStore *storage.Reports
	if opts.Report.Enabled {
		reportsStore, err = storage.NewReports(ctx, dataDB)
		if err != nil {
			return nil, fmt.Errorf("can't make reports store, %w", err)
		}
	}

	detectedSpamStore, err := storage.NewDetectedSpam(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make detected spam store, %w", err)
	}

	assembly := &runtimeAssembly{
		DataDB:              dataDB,
		SpamBot:             spamBot,
		SpamLogger:          spamLogger,
		LoggerWriter:        loggerWr,
		Locator:             locator,
		RuleSets:            ruleSets,
		IncomingEventsStore: incomingEventsStore,
		ReportsStore:        reportsStore,
		DetectedSpamStore:   detectedSpamStore,
		Web: webRuntimeAssembly{
			Detector:      spamBot.Detector,
			SpamFilter:    spamBot,
			Locator:       locator,
			StorageEngine: dataDB,
		},
	}

	return assembly, nil
}

func bootstrapRuleSet(opts options) rules.RuleSet {
	return rules.RuleSet{
		WorkspaceID: opts.InstanceID,
		Source:      "bootstrap",
		Meta: rules.MetaRules{
			LinksLimit:      opts.Meta.LinksLimit,
			MentionsLimit:   opts.Meta.MentionsLimit,
			ImageOnly:       opts.Meta.ImageOnly,
			LinksOnly:       opts.Meta.LinksOnly,
			VideosOnly:      opts.Meta.VideosOnly,
			AudiosOnly:      opts.Meta.AudiosOnly,
			ContactOnly:     opts.Meta.ContactOnly,
			Forwarded:       opts.Meta.Forward,
			Keyboard:        opts.Meta.Keyboard,
			UsernameSymbols: opts.Meta.UsernameSymbols,
			Giveaway:        opts.Meta.Giveaway,
		},
		Duplicates: rules.DuplicateRules{
			Threshold: opts.Duplicates.Threshold,
			Window:    opts.Duplicates.Window,
		},
		AbnormalSpacing: rules.AbnormalSpacingRules{
			Enabled:                 opts.AbnormalSpacing.Enabled,
			SpaceRatioThreshold:     opts.AbnormalSpacing.SpaceRatioThreshold,
			ShortWordRatioThreshold: opts.AbnormalSpacing.ShortWordRatioThreshold,
			ShortWordLen:            opts.AbnormalSpacing.ShortWordLen,
			MinWords:                opts.AbnormalSpacing.MinWords,
		},
		Moderation: rules.ModerationRules{
			FirstStrike:  opts.Moderation.FirstStrike,
			SecondStrike: opts.Moderation.SecondStrike,
			SoftBan:      opts.SoftBan,
			DryRun:       opts.Dry,
		},
		Reports: rules.ReportRules{
			Enabled:          opts.Report.Enabled,
			Threshold:        opts.Report.Threshold,
			AutoBanThreshold: opts.Report.AutoBanThreshold,
			RateLimit:        opts.Report.RateLimit,
			RatePeriod:       opts.Report.RatePeriod,
		},
		OpenAI: rules.LLMRules{
			Enabled:            opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "",
			Veto:               opts.OpenAI.Veto,
			Model:              opts.OpenAI.Model,
			HistorySize:        opts.OpenAI.HistorySize,
			CheckShortMessages: opts.OpenAI.CheckShortMessages,
			CustomPrompts:      opts.OpenAI.CustomPrompts,
		},
		Gemini: rules.LLMRules{
			Enabled:            opts.Gemini.Token != "",
			Veto:               opts.Gemini.Veto,
			Model:              opts.Gemini.Model,
			HistorySize:        opts.Gemini.HistorySize,
			CheckShortMessages: opts.Gemini.CheckShortMessages,
			CustomPrompts:      opts.Gemini.CustomPrompts,
		},
	}
}

func makeApprovedUsersStore(ctx context.Context, dataDB *engine.SQL, detector *tgspam.Detector) (int, *storage.ApprovedUsers, error) {
	approvedUsersStore, err := storage.NewApprovedUsers(ctx, dataDB)
	if err != nil {
		return 0, nil, fmt.Errorf("can't make approved users store, %w", err)
	}

	count, err := detector.WithUserStorage(approvedUsersStore)
	if err != nil {
		return 0, nil, fmt.Errorf("can't load approved users, %w", err)
	}
	return count, approvedUsersStore, nil
}

func (a *runtimeAssembly) makeTelegramListener(opts options, tbAPI *tbapi.BotAPI) *events.TelegramListener {
	listener := &events.TelegramListener{
		TbAPI:               tbAPI,
		BotUsername:         tbAPI.Self.UserName,
		InstanceID:          opts.InstanceID,
		Group:               opts.Telegram.Group,
		IdleDuration:        opts.Telegram.IdleDuration,
		SuperUsers:          opts.SuperUsers,
		Bot:                 a.SpamBot,
		StartupMsg:          opts.Message.Startup,
		WarnMsg:             opts.Message.Warn,
		NoSpamReply:         opts.NoSpamReply,
		SuppressJoinMessage: opts.SuppressJoinMessage,
		DeleteJoinMessages:  opts.Delete.JoinMessages,
		DeleteLeaveMessages: opts.Delete.LeaveMessages,
		SpamLogger:          a.SpamLogger,
		AdminGroup:          opts.AdminGroup,
		TestingIDs:          opts.TestingIDs,
		Locator:             a.Locator,
		IncomingEvents:      a.IncomingEventsStore,
		DetectedSpamCounter: a.DetectedSpamStore,
		ModerationConfig: events.ModerationConfig{
			FirstStrike:  opts.Moderation.FirstStrike,
			SecondStrike: opts.Moderation.SecondStrike,
		},
		ReportConfig: events.ReportConfig{
			Storage:          a.ReportsStore,
			Enabled:          opts.Report.Enabled,
			Threshold:        opts.Report.Threshold,
			AutoBanThreshold: opts.Report.AutoBanThreshold,
			RateLimit:        opts.Report.RateLimit,
			RatePeriod:       opts.Report.RatePeriod,
		},
		TrainingMode:            opts.Training,
		SoftBanMode:             opts.SoftBan,
		DisableAdminSpamForward: opts.DisableAdminSpamForward,
		Dry:                     opts.Dry,
		AggressiveCleanup:       opts.AggressiveCleanup,
		AggressiveCleanupLimit:  opts.AggressiveCleanupLimit,
	}
	a.TelegramListener = listener
	a.Web.BotUsername = listener.BotUsername
	return listener
}

func logListenerConfig(listener *events.TelegramListener) {
	if listener.DeleteJoinMessages {
		log.Print("[INFO] delete join messages enabled")
	}
	if listener.DeleteLeaveMessages {
		log.Print("[INFO] delete leave messages enabled")
	}

	log.Printf("[DEBUG] telegram listener config: {bot: %s, group: %s, idle: %v, super: %v, admin: %s, "+
		"testing: %v, no-reply: %v, suppress: %v, dry: %v, training: %v}",
		listener.BotUsername, listener.Group, listener.IdleDuration, listener.SuperUsers,
		listener.AdminGroup, listener.TestingIDs, listener.NoSpamReply, listener.SuppressJoinMessage,
		listener.Dry, listener.TrainingMode)
}

func activateWebRuntime(ctx context.Context, opts options, web webRuntimeAssembly, dmUsersProvider webapi.DMUsersProvider) error {
	return activateServer(ctx, opts, web, dmUsersProvider)
}

func (a *runtimeAssembly) close() {
	if a.LoggerWriter != nil {
		_ = a.LoggerWriter.Close()
	}
	if a.DataDB != nil {
		_ = a.DataDB.Close()
	}
}
