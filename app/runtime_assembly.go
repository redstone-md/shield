package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/controlplane"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/feedback"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/app/webapi"
	"github.com/umputun/tg-spam/lib/tgspam"
)

type runtimeAssembly struct {
	DataDB                 *engine.SQL
	Detector               *tgspam.Detector
	SpamBot                *bot.SpamFilter
	SpamLogger             events.SpamLogger
	LoggerWriter           io.Closer
	Locator                *storage.Locator
	RuleSets               *storage.RuleSets
	ActiveRuleSet          rules.RuleSet
	IncomingEventsStore    *storage.IncomingEvents
	ModerationActionsStore *storage.ModerationActions
	ReportsStore           *storage.Reports
	DetectedSpamStore      *storage.DetectedSpam
	WorkspacesStore        *storage.Workspaces
	TenantsStore           *storage.Tenants
	WorkspaceService       *controlplane.WorkspaceService
	RoleAuthorizer         *controlplane.RoleAuthorizer
	RuleSetService         *controlplane.RuleSetService
	ApprovedUsersService   *controlplane.ApprovedUsersService
	DictionaryService      *controlplane.DictionaryService
	DetectedSpamService    *controlplane.DetectedSpamService
	TelegramListener       *events.TelegramListener
	AuditService           *audit.Service
	AppealService          *audit.AppealService
	FeedbackService        *feedback.Service
	ReviewService          *feedback.ReviewService
	OnboardingSvc          *controlplane.OnboardingService
	RestoreSvc             *storage.RestoreService
	UsageMetering          *storage.UsageMetering
	RetentionSvc           *storage.RetentionService
	Metrics                *observability.Metrics
	SlowPathEngine         *slowpath.Engine
	SlowPathChatEngine     *slowpath.Engine
	AutoLearner            events.AutoLearner
	Web                    webRuntimeAssembly
}

type webRuntimeAssembly struct {
	Detector             webapi.Detector
	SpamFilter           webapi.SpamFilter
	Locator              webapi.Locator
	StorageEngine        webapi.StorageEngine
	RuleSetService       *controlplane.RuleSetService
	ApprovedUsersService *controlplane.ApprovedUsersService
	DictionaryService    *controlplane.DictionaryService
	DetectedSpamService  *controlplane.DetectedSpamService
	RoleAuthorizer       *controlplane.RoleAuthorizer
	TenantStatusProvider webapi.TenantStatusProvider
	AuditService         *audit.Service
	AppealService        *audit.AppealService
	FeedbackService      *feedback.Service
	ReviewService        *feedback.ReviewService
	KnowledgeService     *feedback.KnowledgeService
	OnboardingProvider   webapi.OnboardingService
	RestoreProvider      webapi.RestoreService
	Metrics              *observability.Metrics
	BotUsername          string
}

func assembleRuntime(ctx context.Context, opts options) (*runtimeAssembly, error) {
	dataDB, err := makeDB(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("can't make db, %w", err)
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

	ruleSets, err := storage.NewRuleSets(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make rule sets store, %w", err)
	}
	if _, err = ruleSets.EnsureBootstrap(ctx, bootstrapRuleSet(opts)); err != nil {
		return nil, fmt.Errorf("can't bootstrap rule set, %w", err)
	}
	activeRuleSet, err := ruleSets.Active(ctx, opts.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("can't load active rule set, %w", err)
	}
	if backfilled, changed := backfillRuleSetSchema(activeRuleSet, opts); changed {
		log.Printf("[INFO] backfilling rule set schema to version %d", rules.CurrentSchemaVersion)
		if _, err = ruleSets.Update(ctx, backfilled); err != nil {
			return nil, fmt.Errorf("can't backfill rule set schema, %w", err)
		}
		activeRuleSet = backfilled
	}
	applyExplicitRuleSetOverrides(&activeRuleSet, opts)

	detector := makeDetectorWithRuleSet(opts, activeRuleSet)
	slowPathEngine := makeSlowPathEngine(opts, activeRuleSet)
	slowPathChatEngine := makeSlowPathChatEngine(opts)
	spamBot, err := makeSpamBot(ctx, opts, activeRuleSet, dataDB, detector)
	if err != nil {
		return nil, fmt.Errorf("can't make spam bot, %w", err)
	}

	if opts.Convert == "only" {
		log.Print("[WARN] convert only mode, converting text samples and exit")
		return &runtimeAssembly{
			DataDB:        dataDB,
			SpamBot:       spamBot,
			SpamLogger:    spamLogger,
			LoggerWriter:  loggerWr,
			RuleSets:      ruleSets,
			ActiveRuleSet: activeRuleSet,
		}, nil
	}

	count, approvedUsersStore, err := makeApprovedUsersStore(ctx, dataDB, detector)
	if err != nil {
		return nil, err
	}
	approvedUsersSvc := controlplane.NewApprovedUsersService(approvedUsersStore, detector)
	log.Printf("[DEBUG] approved users loaded: %d", count)

	dictionaryStore, err := storage.NewDictionary(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make dictionary store, %w", err)
	}
	dictSvc := controlplane.NewDictionaryService(dictionaryStore, spamBot)

	locator, err := storage.NewLocator(ctx, opts.HistoryDuration, opts.HistoryMinSize, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make locator, %w", err)
	}

	incomingEventsStore, err := storage.NewIncomingEvents(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make incoming events store, %w", err)
	}
	moderationActionsStore, err := storage.NewModerationActions(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make moderation actions store, %w", err)
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
	detectedSpamSvc := controlplane.NewDetectedSpamService(detectedSpamStore)

	workspacesStore, err := storage.NewWorkspaces(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make workspaces store, %w", err)
	}
	workspaceService := controlplane.NewWorkspaceService(workspacesStore)
	if _, err = workspaceService.EnsureDefaultWorkspace(ctx, controlplane.WorkspaceBootstrap{
		Name:    opts.InstanceID,
		OwnerID: "tg-spam",
	}); err != nil {
		return nil, fmt.Errorf("can't bootstrap workspace, %w", err)
	}
	roleAuthorizer := controlplane.NewRoleAuthorizer(workspacesStore)
	ruleSetService := controlplane.NewRuleSetService(ruleSets, opts.InstanceID)

	tenantsStore, err := storage.NewTenants(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make tenants store, %w", err)
	}
	if err = tenantsStore.BootstrapDefault(ctx, opts.InstanceID, opts.InstanceID, "tg-spam"); err != nil {
		return nil, fmt.Errorf("can't bootstrap default tenant, %w", err)
	}

	incidentsStore, err := storage.NewIncidentStorage(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make incidents store, %w", err)
	}
	appealsStore, err := storage.NewAppealStorage(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make appeals store, %w", err)
	}
	auditSvc := audit.NewService(incidentsStore)
	appealSvc := audit.NewAppealService(appealsStore, incidentsStore, nil)

	labelsStore, err := storage.NewLabelStorage(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make labels store, %w", err)
	}
	candidatesStore, err := storage.NewCandidateStorage(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make candidates store, %w", err)
	}
	knowledgeStore, err := storage.NewKnowledgeSnapshotStorage(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make knowledge snapshots store, %w", err)
	}
	samplesStore, err := storage.NewSamples(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make samples store, %w", err)
	}
	feedbackSvc := feedback.NewService(labelsStore,
		&sampleAdderAdapter{samples: samplesStore},
		&spamTextProviderAdapter{store: detectedSpamStore},
	)
	reviewSvc := feedback.NewReviewService(candidatesStore, &dictAdderAdapter{dict: dictionaryStore}, feedbackSvc)
	knowledgeSvc := feedback.NewKnowledgeService(knowledgeStore,
		&knowledgeDictAdapter{dict: dictionaryStore},
		&knowledgeSamplesAdapter{samples: samplesStore},
		&stopPhraseRestorerAdapter{dict: dictionaryStore},
	)

	autoLearner := &autoLearnerAdapter{
		samples:   &sampleAdderAdapter{samples: samplesStore},
		reviewSvc: reviewSvc,
	}

	usageMetering, err := storage.NewUsageMetering(ctx, dataDB)
	if err != nil {
		return nil, fmt.Errorf("can't make usage metering store, %w", err)
	}

	appealSvc.SetFeedbackLabeler(feedbackSvc)

	onboardingSvc := controlplane.NewOnboardingService(tenantsStore, workspacesStore, ruleSets, ruleSetService.Cache())
	restoreSvc := storage.NewRestoreService(dataDB)
	metrics := observability.NewMetrics()

	assembly := &runtimeAssembly{
		DataDB:                 dataDB,
		Detector:               detector,
		SpamBot:                spamBot,
		SpamLogger:             spamLogger,
		LoggerWriter:           loggerWr,
		Locator:                locator,
		RuleSets:               ruleSets,
		ActiveRuleSet:          activeRuleSet,
		IncomingEventsStore:    incomingEventsStore,
		ModerationActionsStore: moderationActionsStore,
		ReportsStore:           reportsStore,
		DetectedSpamStore:      detectedSpamStore,
		WorkspacesStore:        workspacesStore,
		WorkspaceService:       workspaceService,
		TenantsStore:           tenantsStore,
		RoleAuthorizer:         roleAuthorizer,
		RuleSetService:         ruleSetService,
		ApprovedUsersService:   approvedUsersSvc,
		DictionaryService:      dictSvc,
		DetectedSpamService:    detectedSpamSvc,
		AppealService:          appealSvc,
		FeedbackService:        feedbackSvc,
		ReviewService:          reviewSvc,
		AutoLearner:            autoLearner,
		UsageMetering:          usageMetering,
		RetentionSvc: storage.NewRetentionService(dataDB, storage.RetentionConfig{
			IncidentsTTL:         opts.Retention.IncidentsTTL,
			AppealsTTL:           opts.Retention.AppealsTTL,
			DetectedSpamTTL:      opts.Retention.DetectedSpamTTL,
			LabelsTTL:            opts.Retention.LabelsTTL,
			CandidatesTTL:        opts.Retention.CandidatesTTL,
			IncomingEventsTTL:    opts.Retention.IncomingEventsTTL,
			ModerationActionsTTL: opts.Retention.ModerationActionsTTL,
			UsageCountersTTL:     opts.Retention.UsageCountersTTL,
			Interval:             opts.Retention.Interval,
		}),
		Metrics:            metrics,
		SlowPathEngine:     slowPathEngine,
		SlowPathChatEngine: slowPathChatEngine,
		OnboardingSvc:      onboardingSvc,
		RestoreSvc:         restoreSvc,
		Web: webRuntimeAssembly{
			Detector:             detector,
			SpamFilter:           spamBot,
			Locator:              locator,
			StorageEngine:        dataDB,
			RuleSetService:       ruleSetService,
			RoleAuthorizer:       roleAuthorizer,
			ApprovedUsersService: approvedUsersSvc,
			DictionaryService:    dictSvc,
			DetectedSpamService:  detectedSpamSvc,
			TenantStatusProvider: tenantStatusAdapter{inner: tenantsStore},
			AuditService:         auditSvc,
			AppealService:        appealSvc,
			FeedbackService:      feedbackSvc,
			ReviewService:        reviewSvc,
			KnowledgeService:     knowledgeSvc,
			OnboardingProvider:   &onboardingAdapter{inner: onboardingSvc},
			RestoreProvider:      &restoreProviderAdapter{svc: restoreSvc},
			Metrics:              metrics,
		},
	}

	return assembly, nil
}

func bootstrapRuleSet(opts options) rules.RuleSet {
	return rules.RuleSet{
		WorkspaceID:   opts.InstanceID,
		Source:        "bootstrap",
		SchemaVersion: rules.CurrentSchemaVersion,
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
			FirstStrike:        opts.Moderation.FirstStrike,
			SecondStrike:       opts.Moderation.SecondStrike,
			WarnStrikes:        opts.Moderation.WarnStrikes,
			SoftBan:            opts.SoftBan,
			DryRun:             opts.Dry,
			WarnDeleteDuration: opts.Moderation.WarnDeleteDuration,
		},
		Reports: rules.ReportRules{
			Enabled:          opts.Report.Enabled,
			Threshold:        opts.Report.Threshold,
			AutoBanThreshold: opts.Report.AutoBanThreshold,
			RateLimit:        opts.Report.RateLimit,
			RatePeriod:       opts.Report.RatePeriod,
		},
		Detection: rules.DetectionRules{
			MaxEmoji:            opts.MaxEmoji,
			MinMsgLen:           opts.MinMsgLen,
			SimilarityThreshold: opts.SimilarityThreshold,
			MinSpamProbability:  opts.MinSpamProbability,
			MultiLangWords:      opts.MultiLangWords,
			CasEnabled:          opts.CAS.API != "",
			HistorySize:         opts.HistoryMinSize,
			FirstMessagesCount:  opts.FirstMessagesCount,
			ParanoidMode:        opts.ParanoidMode,
		},
		LLM: rules.LLMCommonRules{
			Mode:      opts.LLM.Mode,
			Consensus: opts.LLM.Consensus,
		},
		OpenAI: rules.LLMRules{
			Enabled:            opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "",
			Veto:               opts.OpenAI.Veto,
			Model:              opts.OpenAI.Model,
			VisionModel:        opts.OpenAI.VisionModel,
			Prompt:             opts.OpenAI.Prompt,
			HistorySize:        opts.OpenAI.HistorySize,
			CheckShortMessages: opts.OpenAI.CheckShortMessages,
			CustomPrompts:      opts.OpenAI.CustomPrompts,
		},
		Gemini: rules.LLMRules{
			Enabled:            opts.Gemini.Token != "",
			Veto:               opts.Gemini.Veto,
			Model:              opts.Gemini.Model,
			VisionModel:        opts.Gemini.VisionModel,
			Prompt:             opts.Gemini.Prompt,
			HistorySize:        opts.Gemini.HistorySize,
			CheckShortMessages: opts.Gemini.CheckShortMessages,
			CustomPrompts:      opts.Gemini.CustomPrompts,
		},
	}
}

// backfillRuleSetSchema seeds new fields into a ruleset persisted before the
// current schema version. Identity and versioning fields are preserved; only the
// detection/LLM/prompt fields are seeded from env-derived defaults. The second
// return value reports whether the ruleset was modified.
func backfillRuleSetSchema(rs rules.RuleSet, opts options) (rules.RuleSet, bool) {
	if rs.SchemaVersion >= rules.CurrentSchemaVersion {
		return rs, false
	}
	seed := bootstrapRuleSet(opts)
	rs.SchemaVersion = rules.CurrentSchemaVersion
	rs.Detection = seed.Detection
	rs.LLM = seed.LLM
	rs.OpenAI.Prompt = seed.OpenAI.Prompt
	rs.OpenAI.VisionModel = seed.OpenAI.VisionModel
	rs.Gemini.Prompt = seed.Gemini.Prompt
	rs.Gemini.VisionModel = seed.Gemini.VisionModel
	return rs, true
}

func makeApprovedUsersStore(
	ctx context.Context, dataDB *engine.SQL, detector *tgspam.Detector,
) (int, *storage.ApprovedUsers, error) {
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
		TenantID:            opts.InstanceID,
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
		ModerationActions:   a.ModerationActionsStore,
		DetectedSpamCounter: a.DetectedSpamStore,
		RuleSetVersion:      a.ActiveRuleSet.Version,
		ModerationConfig: events.ModerationConfig{
			FirstStrike:        a.ActiveRuleSet.Moderation.FirstStrike,
			SecondStrike:       a.ActiveRuleSet.Moderation.SecondStrike,
			WarnStrikes:        a.ActiveRuleSet.Moderation.WarnStrikes,
			WarnDeleteDuration: a.ActiveRuleSet.Moderation.WarnDeleteDuration,
		},
		ReportConfig: events.ReportConfig{
			Storage:          a.ReportsStore,
			Enabled:          a.ActiveRuleSet.Reports.Enabled,
			Threshold:        a.ActiveRuleSet.Reports.Threshold,
			AutoBanThreshold: a.ActiveRuleSet.Reports.AutoBanThreshold,
			RateLimit:        a.ActiveRuleSet.Reports.RateLimit,
			RatePeriod:       a.ActiveRuleSet.Reports.RatePeriod,
		},
		TrainingMode:            opts.Training,
		SoftBanMode:             a.ActiveRuleSet.Moderation.SoftBan,
		DisableAdminSpamForward: opts.DisableAdminSpamForward,
		Dry:                     a.ActiveRuleSet.Moderation.DryRun,
		AggressiveCleanup:       opts.AggressiveCleanup,
		AggressiveCleanupLimit:  opts.AggressiveCleanupLimit,
		DeleteGuestBots:         opts.DeleteGuestBots,
		BotWhitelist:            opts.BotWhitelist,
	}
	if a.AuditService != nil {
		incidentCreator := events.NewIncidentAdapter(a.AuditService)
		listener.AuditWriter = events.NewDefaultAuditWriter(a.SpamLogger, a.Locator, incidentCreator)
		listener.IncidentCreator = incidentCreator
	}
	if a.UsageMetering != nil {
		listener.UsageMeter = &usageMeterAdapter{store: a.UsageMetering}
	}
	if a.Metrics != nil {
		listener.MetricsRecorder = a.Metrics
	}
	if a.SlowPathEngine != nil {
		listener.SlowPathEngine = a.SlowPathEngine
		listener.SlowPathEnabled = true
	}
	if a.SlowPathChatEngine != nil {
		listener.SlowPathChatEngine = a.SlowPathChatEngine
	}
	if a.ReviewService != nil {
		listener.CandidateGenerator = &candidateGeneratorAdapter{svc: a.ReviewService}
	}
	if a.AutoLearner != nil {
		listener.AutoLearner = a.AutoLearner
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

	debugLogFields("telegram listener config", loggableTelegramListenerConfig(listener))
}

func activateWebRuntime(ctx context.Context, opts options, web webRuntimeAssembly, dmUsersProvider webapi.DMUsersProvider) error {
	return activateServer(ctx, opts, web, dmUsersProvider)
}

func applyLiveReload(a *runtimeAssembly, opts options, rs rules.RuleSet) {
	log.Printf("[INFO] rule set changed: version=%d, applying live reload", rs.Version)
	applyExplicitRuleSetOverrides(&rs, opts)

	if a.TelegramListener != nil {
		a.TelegramListener.ApplyRuleSet(rs)
	}

	if a.Detector != nil {
		cfg := buildDetectorConfig(opts, rs)
		a.Detector.UpdateConfig(cfg)
		a.Detector.ReplaceMetaChecks(buildMetaChecks(rs, rs.Detection.MinMsgLen)...)
		applyLLMCheckers(a.Detector, opts, rs)
	}

	if a.SlowPathEngine != nil {
		applySlowPathPrompts(a.SlowPathEngine, rs)
	}

	if a.SpamBot != nil {
		a.SpamBot.ApplyRuleSet(rs)
	}
	a.ActiveRuleSet = rs

	log.Printf("[INFO] live reload applied: version=%d", rs.Version)
}

func (a *runtimeAssembly) wireLiveReload(opts options) {
	a.RuleSetService.OnChange(func(rs rules.RuleSet) {
		applyLiveReload(a, opts, rs)
	})

	if a.ApprovedUsersService != nil {
		a.ApprovedUsersService.OnChange(func() {
			log.Printf("[INFO] approved users changed, invalidating rule set cache")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a.RuleSetService.Invalidate(ctx)
		})
	}

	if a.DictionaryService != nil {
		a.DictionaryService.OnChange(func() {
			log.Printf("[INFO] dictionary changed, invalidating rule set cache")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a.RuleSetService.Invalidate(ctx)
		})
	}

	if a.DetectedSpamService != nil {
		a.DetectedSpamService.OnChange(func() {
			log.Printf("[INFO] detected spam changed")
		})
	}
}

func (a *runtimeAssembly) close() {
	if a.LoggerWriter != nil {
		_ = a.LoggerWriter.Close()
	}
	if a.DataDB != nil {
		_ = a.DataDB.Close()
	}
}
