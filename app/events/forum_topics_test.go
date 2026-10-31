package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/bot"
)

func sentThreadID(t *testing.T, c tbapi.Chattable) int {
	t.Helper()
	msg, ok := c.(tbapi.MessageConfig)
	require.True(t, ok)
	return msg.MessageThreadID
}

func TestTopicThreadID(t *testing.T) {
	tests := []struct {
		name string
		msg  *tbapi.Message
		want int
	}{
		{name: "nil message"},
		{name: "plain message", msg: &tbapi.Message{}},
		{name: "topic message", msg: &tbapi.Message{MessageThreadID: 7, IsTopicMessage: true}, want: 7},
		{name: "reply in a discussion group", msg: &tbapi.Message{MessageThreadID: 7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, topicThreadID(tt.msg))
		})
	}
}

func TestTransformKeepsTopic(t *testing.T) {
	msg := transform(&tbapi.Message{Chat: tbapi.Chat{ID: 1}, From: &tbapi.User{ID: 2}, MessageThreadID: 7, IsTopicMessage: true})
	assert.Equal(t, 7, msg.MessageThreadID)
}

func TestExecutorSendsToTopic(t *testing.T) {
	api := okTbAPI()
	exec := newTelegramActionExecutor(api, nil, nil)

	require.NoError(t, exec.WarnUser(context.Background(), warnRequest{chatID: 1, threadID: 7, text: "warn"}))
	require.NoError(t, exec.PostBanMessage(context.Background(), banMessageRequest{chatID: 1, threadID: 8, text: "ban"}))

	require.Len(t, api.SendCalls(), 2)
	assert.Equal(t, 7, sentThreadID(t, api.SendCalls()[0].C))
	assert.Equal(t, 8, sentThreadID(t, api.SendCalls()[1].C))
}

func TestPipelineWarnsInTopic(t *testing.T) {
	spy := &actionExecutorSpy{}
	l := TelegramListener{TbAPI: okTbAPI(), ActionExecutor: spy, AuditWriter: &auditWriterSpy{}}
	pc := pipelineContext{
		msg:      &bot.Message{ID: 77, MessageThreadID: 7, From: bot.User{ID: 42}},
		resp:     bot.Response{Send: true},
		fromChat: 1, spamUserID: 42,
	}

	require.NoError(t, l.processWarn(context.Background(), pc))

	require.Len(t, spy.warnCalls, 1)
	assert.Equal(t, 7, spy.warnCalls[0].threadID)
}

func TestPipelinePostsBanNoticeInTopic(t *testing.T) {
	spy := &actionExecutorSpy{}
	l := TelegramListener{TbAPI: okTbAPI(), ActionExecutor: spy}
	pc := pipelineContext{msg: &bot.Message{ID: 77, MessageThreadID: 7}, fromChat: 1, outcome: PolicyOutcome{Duration: time.Hour}}

	l.postBanGroupMessage(context.Background(), pc, 0)

	require.Len(t, spy.postBanCalls, 1)
	assert.Equal(t, 7, spy.postBanCalls[0].threadID)
}

func TestBotResponseGoesToTopic(t *testing.T) {
	api := okTbAPI()
	l := TelegramListener{TbAPI: api}

	require.NoError(t, l.sendBotResponse(bot.Response{Send: true, Text: "hi", MessageThreadID: 7}, 1, NotificationDefault))

	require.Len(t, api.SendCalls(), 1)
	assert.Equal(t, 7, sentThreadID(t, api.SendCalls()[0].C))
}

func TestReportStatusGoesToTopic(t *testing.T) {
	api := okTbAPI()
	r := userReports{tbAPI: api}

	r.notifyPrimaryChat(reportOutcomeAccepted, 0, false, 1, 7)

	require.Len(t, api.SendCalls(), 1)
	assert.Equal(t, 7, sentThreadID(t, api.SendCalls()[0].C))
}

func TestAdminWarnGoesToTopic(t *testing.T) {
	api := okTbAPI()
	a := admin{tbAPI: api}

	err := a.sendWarnMessage(context.Background(), &tbapi.Message{Chat: tbapi.Chat{ID: 1}, MessageThreadID: 7, IsTopicMessage: true}, "warn")

	require.NoError(t, err)
	require.Len(t, api.SendCalls(), 1)
	assert.Equal(t, 7, sentThreadID(t, api.SendCalls()[0].C))
}
