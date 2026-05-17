package events

import (
	"context"
	"errors"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/events/mocks"
)

type fakeAppealFiler struct {
	incident    audit.Incident
	incidentErr error
	existing    audit.Appeal
	existingErr error
	submitted   audit.Appeal
	submitErr   error
	submitCalls []int64
}

func (f *fakeAppealFiler) GetIncident(_ context.Context, _ int64) (audit.Incident, error) {
	return f.incident, f.incidentErr
}

func (f *fakeAppealFiler) GetForIncident(_ context.Context, _ int64) (audit.Appeal, error) {
	return f.existing, f.existingErr
}

func (f *fakeAppealFiler) Submit(_ context.Context, incidentID, _ int64, _, _ string) (audit.Appeal, error) {
	f.submitCalls = append(f.submitCalls, incidentID)
	return f.submitted, f.submitErr
}

func newAppealTestMessage(userID int64, text string) *tbapi.Message {
	return &tbapi.Message{
		Chat: tbapi.Chat{ID: userID, Type: "private"},
		From: &tbapi.User{ID: userID, FirstName: "Spammer"},
		Text: text,
	}
}

func TestAppealHandler_Handle(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		filer      *fakeAppealFiler
		wantReply  string
		wantSubmit bool
		wantAdmin  bool
	}{
		{
			name:      "garbage payload",
			payload:   "not-a-number",
			filer:     &fakeAppealFiler{},
			wantReply: "Неверная ссылка.",
		},
		{
			name:      "incident not found",
			payload:   "10",
			filer:     &fakeAppealFiler{incidentErr: errors.New("missing")},
			wantReply: "Инцидент не найден.",
		},
		{
			name:      "wrong user",
			payload:   "10",
			filer:     &fakeAppealFiler{incident: audit.Incident{ID: 10, SpamUserID: 999, Status: audit.IncidentStatusOpen}},
			wantReply: "Эта ссылка не для вас.",
		},
		{
			name:    "closed incident",
			payload: "10",
			filer: &fakeAppealFiler{
				incident: audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusClosed},
			},
			wantReply: "Наказание уже неактивно.",
		},
		{
			name:    "appeal already filed",
			payload: "10",
			filer: &fakeAppealFiler{
				incident: audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusAppealed},
				existing: audit.Appeal{ID: 1, Status: audit.AppealNew},
			},
			wantReply: "Апелляция уже подана, ожидайте решения модераторов.",
		},
		{
			name:    "appeal already reviewed",
			payload: "10",
			filer: &fakeAppealFiler{
				incident: audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusAppealed},
				existing: audit.Appeal{ID: 1, Status: audit.AppealAccepted},
			},
			wantReply: "Апелляция уже рассмотрена.",
		},
		{
			name:    "success",
			payload: "10",
			filer: &fakeAppealFiler{
				incident:    audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusOpen, ReasonText: "regex"},
				existingErr: errors.New("no appeal yet"),
				submitted:   audit.Appeal{ID: 88},
			},
			wantReply:  "✅ Апелляция подана, ожидайте решения модераторов.",
			wantSubmit: true,
			wantAdmin:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []tbapi.MessageConfig
			mockAPI := &mocks.TbAPIMock{
				SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
					sent = append(sent, c.(tbapi.MessageConfig))
					return tbapi.Message{}, nil
				},
			}
			h := newAppealHandler(mockAPI, tt.filer, 12345)

			err := h.Handle(context.Background(), newAppealTestMessage(555, "/start "+tt.payload), tt.payload)
			require.NoError(t, err)

			require.NotEmpty(t, sent)
			assert.Equal(t, tt.wantReply, sent[0].Text)
			assert.Equal(t, tt.wantSubmit, len(tt.filer.submitCalls) == 1)
			if tt.wantAdmin {
				require.Len(t, sent, 2)
				assert.Equal(t, int64(12345), sent[1].ChatID)
				markup, ok := sent[1].ReplyMarkup.(tbapi.InlineKeyboardMarkup)
				require.True(t, ok)
				require.NotNil(t, markup.InlineKeyboard[0][0].CallbackData)
				assert.Equal(t, "AA88", *markup.InlineKeyboard[0][0].CallbackData)
				require.NotNil(t, markup.InlineKeyboard[0][1].CallbackData)
				assert.Equal(t, "AR88", *markup.InlineKeyboard[0][1].CallbackData)
			}
		})
	}
}
