package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
)

type incomingEventProcessor interface {
	Process(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error
}

type pendingIncomingEvent struct {
	update tbapi.Update
	result chan error
}

type listenerPipeline struct {
	worker   sync.WaitGroup
	pending  map[string]pendingIncomingEvent
	mu       sync.Mutex
	eventID  uint64
	running  bool
	ownQueue bool
}

type listenerEventProcessor struct {
	listener *TelegramListener
}

func (p listenerEventProcessor) Process(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	return p.listener.processQueuedEvent(ctx, event, update)
}

func (l *TelegramListener) procEvents(update tbapi.Update) error {
	return l.procEventsWithContext(context.Background(), update)
}

func (l *TelegramListener) procEventsWithContext(ctx context.Context, update tbapi.Update) error {
	if update.Message == nil {
		return nil
	}

	// intercept private (DM) messages before any other processing.
	// stores the sender info for the admin UI and silently drops the message.
	if update.Message.Chat.Type == "private" {
		if update.Message.From == nil {
			return nil
		}
		from := update.Message.From
		displayName := strings.TrimSpace(from.FirstName + " " + from.LastName)
		l.dmUsers.Add(DMUser{
			UserID:      from.ID,
			UserName:    from.UserName,
			DisplayName: displayName,
			Timestamp:   time.Now(),
		})
		return nil
	}

	fromChat := update.Message.Chat.ID
	if !l.isChatAllowed(fromChat) {
		return nil
	}

	msg := transform(update.Message)
	if strings.TrimSpace(msg.Text) == "" && msg.Image == nil && !msg.WithVideoNote && !msg.WithVideo && !msg.WithForward {
		return nil
	}

	event := l.makeIncomingEvent(msg)
	return l.enqueueIncomingEvent(ctx, event, update)
}

func (l *TelegramListener) ensurePipeline() {
	l.pipeline.mu.Lock()
	defer l.pipeline.mu.Unlock()
	if l.pipeline.running {
		return
	}
	if l.Queue == nil {
		l.Queue = moderation.NewInMemoryQueue(100)
		l.pipeline.ownQueue = true
	} else {
		l.pipeline.ownQueue = false
	}
	if l.processor == nil {
		l.processor = listenerEventProcessor{listener: l}
	}
	if l.ActionExecutor == nil {
		exec := newTelegramActionExecutor(l.TbAPI, l.Dry, l.TrainingMode, l.SuperUsers)
		l.ActionExecutor = exec
	}
	l.pipeline.pending = make(map[string]pendingIncomingEvent)
	l.pipeline.running = true
	l.pipeline.worker.Add(1)
	go l.runQueueWorker()
}

func (l *TelegramListener) enqueueIncomingEvent(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	l.ensurePipeline()

	resultCh := make(chan error, 1)
	l.pipeline.mu.Lock()
	l.pipeline.pending[event.EventID] = pendingIncomingEvent{update: update, result: resultCh}
	l.pipeline.mu.Unlock()

	if err := l.Queue.Publish(ctx, event); err != nil {
		l.pipeline.mu.Lock()
		delete(l.pipeline.pending, event.EventID)
		l.pipeline.mu.Unlock()
		return fmt.Errorf("publish incoming event %s: %w", event.EventID, err)
	}

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *TelegramListener) runQueueWorker() {
	defer l.pipeline.worker.Done()

	for event := range l.Queue.Consume() {
		l.pipeline.mu.Lock()
		pending, ok := l.pipeline.pending[event.EventID]
		if ok {
			delete(l.pipeline.pending, event.EventID)
		}
		l.pipeline.mu.Unlock()
		if !ok {
			log.Printf("[WARN] dropped moderation event %s without pending telegram update", event.EventID)
			continue
		}

		err := l.processor.Process(context.Background(), event, pending.update)
		pending.result <- err
		close(pending.result)
	}
}

func (l *TelegramListener) shutdownPipeline() {
	l.pipeline.mu.Lock()
	if !l.pipeline.running || l.Queue == nil {
		l.pipeline.mu.Unlock()
		return
	}
	queue := l.Queue
	ownQueue := l.pipeline.ownQueue
	l.pipeline.running = false
	l.pipeline.pending = nil
	l.pipeline.mu.Unlock()

	queue.Close()
	l.pipeline.worker.Wait()
	if ownQueue {
		l.Queue = nil
	}
}

func (l *TelegramListener) makeIncomingEvent(msg *bot.Message) moderation.IncomingEvent {
	subjectID := msg.From.ID
	subjectUserName := msg.From.Username
	if msg.SenderChat.ID != 0 {
		subjectID = msg.SenderChat.ID
		subjectUserName = msg.SenderChat.UserName
	}

	return moderation.IncomingEvent{
		EventID:       l.nextEventID(),
		CorrelationID: l.currentCorrelationID(),
		TenantID:      l.InstanceID,
		Source:        "telegram.update",
		ChatID:        msg.ChatID,
		MessageID:     msg.ID,
		Subject: moderation.Subject{
			ID:       subjectID,
			UserName: subjectUserName,
			IsBot:    msg.From.ID == 136817688,
		},
		Content: moderation.Content{
			Text:       msg.Text,
			Links:      collectLinks(msg),
			HasMedia:   msg.Image != nil || msg.WithVideo || msg.WithVideoNote || msg.WithAudio,
			Attributes: incomingEventAttributes(msg),
		},
		ReceivedAt: msg.Sent.UTC(),
	}
}

func (l *TelegramListener) nextEventID() string {
	seq := atomic.AddUint64(&l.pipeline.eventID, 1)
	return fmt.Sprintf("evt-%s-%d", strings.TrimSpace(l.InstanceID), seq)
}

func (l *TelegramListener) currentCorrelationID() string {
	return fmt.Sprintf("corr-%s", strings.TrimSpace(l.InstanceID))
}

func incomingEventAttributes(msg *bot.Message) map[string]string {
	attrs := map[string]string{
		"with_forward":    strconv.FormatBool(msg.WithForward),
		"with_keyboard":   strconv.FormatBool(msg.WithKeyboard),
		"with_contact":    strconv.FormatBool(msg.WithContact),
		"with_giveaway":   strconv.FormatBool(msg.WithGiveaway),
		"with_video":      strconv.FormatBool(msg.WithVideo),
		"with_video_note": strconv.FormatBool(msg.WithVideoNote),
		"with_audio":      strconv.FormatBool(msg.WithAudio),
	}
	if msg.SenderChat.ID != 0 {
		attrs["sender_chat_id"] = strconv.FormatInt(msg.SenderChat.ID, 10)
		if msg.SenderChat.UserName != "" {
			attrs["sender_chat_username"] = msg.SenderChat.UserName
		}
	}
	return attrs
}

func collectLinks(msg *bot.Message) []string {
	var links []string
	appendLinks := func(entities *[]bot.Entity) {
		if entities == nil {
			return
		}
		for _, entity := range *entities {
			switch entity.Type {
			case "url":
				links = append(links, entity.URL)
			case "text_link":
				if entity.URL != "" {
					links = append(links, entity.URL)
				}
			}
		}
	}

	appendLinks(msg.Entities)
	if msg.Image != nil {
		appendLinks(msg.Image.Entities)
	}
	return links
}

func (l *TelegramListener) processQueuedEvent(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	msgJSON, errJSON := json.Marshal(update.Message)
	if errJSON != nil {
		return fmt.Errorf("failed to marshal update.Message to json: %w", errJSON)
	}

	fromChat := update.Message.Chat.ID
	log.Printf("[DEBUG] event_id=%s correlation_id=%s %s", event.EventID, event.CorrelationID, string(msgJSON))
	msg := transform(update.Message)

	log.Printf("[DEBUG] event_id=%s correlation_id=%s incoming msg: %+v",
		event.EventID, event.CorrelationID, strings.ReplaceAll(msg.Text, "\n", " "))
	log.Printf("[DEBUG] event_id=%s correlation_id=%s incoming msg details: %+v", event.EventID, event.CorrelationID, msg)

	// use channel identity for locator when message is sent on behalf of a channel
	locatorUserID := msg.From.ID
	locatorUserName := msg.From.Username
	if msg.SenderChat.ID != 0 {
		locatorUserID = msg.SenderChat.ID
		locatorUserName = msg.SenderChat.UserName
	}
	if err := l.Locator.AddMessage(ctx, msg.Text, fromChat, locatorUserID, locatorUserName, msg.ID); err != nil {
		log.Printf("[WARN] failed to add message to locator: %v", err)
	}

	// skip spam check for anonymous admin posts from this group or from the linked channel.
	if msg.SenderChat.ID != 0 && (msg.SenderChat.ID == fromChat || msg.SenderChat.ID == l.linkedChannelID) {
		log.Printf("[DEBUG] event_id=%s correlation_id=%s skipping spam check for anonymous admin post from group itself or linked channel",
			event.EventID, event.CorrelationID)
		return nil
	}

	resp := l.Bot.OnMessage(*msg, false)
	if !resp.Send {
		return nil
	}

	if resp.Send && !l.NoSpamReply && !l.TrainingMode {
		if err := l.sendBotResponse(resp, fromChat, NotificationSilent); err != nil {
			log.Printf("[WARN] failed to respond on update, %v", err)
		}
	}

	errs := new(multierror.Error)
	if resp.Send && resp.BanInterval > 0 {
		log.Printf("[DEBUG] event_id=%s correlation_id=%s ban initiated for %+v", event.EventID, event.CorrelationID, resp)
		l.SpamLogger.Save(msg, &resp)
		spamUserID := msg.From.ID
		if msg.SenderChat.ID != 0 {
			spamUserID = msg.SenderChat.ID
		}
		if err := l.Locator.AddSpam(ctx, spamUserID, resp.CheckResults); err != nil {
			log.Printf("[WARN] failed to add spam to locator: %v", err)
		}
		banUserStr := l.getBanUsername(resp, update)

		if l.SuperUsers.IsSuper(msg.From.Username, msg.From.ID) {
			if l.TrainingMode {
				l.adminHandler.ReportBan(banUserStr, msg, resp.BanInterval, l.SoftBanMode)
			}
			log.Printf("[DEBUG] superuser %s requested ban, ignored", banUserStr)
			return nil
		}

		duration, restrict := resp.BanInterval, l.SoftBanMode
		if spamUserID != 0 && l.DetectedSpamCounter != nil {
			if count, countErr := l.DetectedSpamCounter.CountByUserID(ctx, spamUserID); countErr != nil {
				log.Printf("[WARN] failed to count spam strikes for user %d: %v", spamUserID, countErr)
			} else {
				duration, restrict = spamPenalty(count, l.SoftBanMode, l.ModerationConfig)
			}
		}

		banReq := banRequest{duration: duration, userID: resp.User.ID, channelID: resp.ChannelID, userName: banUserStr,
			chatID: fromChat, dry: l.Dry, training: l.TrainingMode, restrict: restrict}
		if err := l.ActionExecutor.ApplyBan(banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban %s: %w", banUserStr, err))
		} else if l.adminChatID != 0 && msg.From.ID != 0 {
			l.adminHandler.ReportBan(banUserStr, msg, duration, restrict)
		}
	}

	if err := l.ActionExecutor.DeleteExtraMessages(resp.CheckResults, msg.From.ID, msg.From.Username, fromChat); err != nil {
		errs = multierror.Append(errs, err)
	}

	canDelete := resp.DeleteReplyTo && resp.ReplyTo != 0 && !l.Dry &&
		!l.SuperUsers.IsSuper(msg.From.Username, msg.From.ID) && !l.TrainingMode
	if canDelete {
		if err := l.ActionExecutor.DeleteMessage(l.chatID, resp.ReplyTo); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("processing events failed: %w", err)
	}
	return nil
}
