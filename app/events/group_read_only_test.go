package events

import (
	"context"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/lib/spamcheck"
)

func okTbAPI() *mocks.TbAPIMock {
	return &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{MessageID: 1}, nil },
	}
}

// modes lists the combinations of dry and training with whether automatic actions may touch the group.
var modes = []struct {
	name          string
	dry, training bool
	allowed       bool
}{
	{name: "normal", allowed: true},
	{name: "dry", dry: true},
	{name: "training", training: true},
}

func TestGroupReadOnlyListener(t *testing.T) {
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			api := okTbAPI()
			l := TelegramListener{TbAPI: api, Dry: m.dry, TrainingMode: m.training, adminChatID: 999}

			l.deleteSystemMessage(10, 123, "join")
			assert.Equal(t, m.allowed, len(api.RequestCalls()) == 1, "system message delete")

			require.NoError(t, l.sendBotResponse(bot.Response{Send: true, Text: "hi"}, 123, NotificationDefault))
			assert.Equal(t, m.allowed, len(api.SendCalls()) == 1, "group response")

			sent := len(api.SendCalls())
			require.NoError(t, l.sendBotResponse(bot.Response{Send: true, Text: "error: x"}, 999, NotificationDefault))
			assert.Len(t, api.SendCalls(), sent+1, "admin chat always gets responses")
		})
	}
}

func TestGroupReadOnlyWarnAndCleanup(t *testing.T) {
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			spy := &actionExecutorSpy{}
			l := TelegramListener{
				TbAPI: okTbAPI(), Dry: m.dry, TrainingMode: m.training,
				ActionExecutor: spy, AuditWriter: &auditWriterSpy{},
			}
			pc := pipelineContext{
				msg: &bot.Message{ID: 77, From: bot.User{ID: 42, Username: "spammer"}},
				resp: bot.Response{
					Send: true, ReplyTo: 77, DeleteReplyTo: true,
					CheckResults: []spamcheck.Response{{Name: "duplicates", Spam: true, ExtraDeleteIDs: []int{78}}},
				},
				fromChat: 123, spamUserID: 42,
			}

			require.NoError(t, l.processWarn(context.Background(), pc))

			if m.allowed {
				assert.Len(t, spy.warnCalls, 1)
				assert.NotEmpty(t, spy.deleteMessageCalls)
				assert.Len(t, spy.deleteExtraCalls, 1)
				return
			}
			assert.Empty(t, spy.warnCalls)
			assert.Empty(t, spy.deleteMessageCalls)
			assert.Empty(t, spy.deleteExtraCalls)
		})
	}
}

func TestGroupReadOnlyReports(t *testing.T) {
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			api := okTbAPI()
			r := userReports{tbAPI: api, dry: m.dry, trainingMode: m.training}

			r.deleteReportCommand(10, 123)
			assert.Equal(t, m.allowed, len(api.RequestCalls()) == 1, "report command delete")

			r.notifyPrimaryChat(reportOutcomeAccepted, 0, false, 123)
			assert.Equal(t, m.allowed, len(api.SendCalls()) == 1, "report status")
		})
	}
}

// explicit admin actions follow dry only: training keeps them real, as confirming a ban does.
func TestAdminActionsFollowDryOnly(t *testing.T) {
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			api := okTbAPI()
			a := admin{tbAPI: api, dry: m.dry, trainingMode: m.training}

			require.NoError(t, a.deleteMessage(context.Background(), 123, 10, "test"))
			assert.Equal(t, !m.dry, len(api.RequestCalls()) == 1, "delete")

			require.NoError(t, a.sendWarnMessage(context.Background(), &tbapi.Message{Chat: tbapi.Chat{ID: 123}}, "warn"))
			assert.Equal(t, !m.dry, len(api.SendCalls()) == 1, "warn")
		})
	}
}
