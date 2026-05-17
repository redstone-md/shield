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

type fakeAppealResolver struct {
	appeal      audit.Appeal
	appealErr   error
	acceptCalls []int64
	rejectCalls []int64
}

func (f *fakeAppealResolver) GetAppeal(_ context.Context, _ int64) (audit.Appeal, error) {
	return f.appeal, f.appealErr
}

func (f *fakeAppealResolver) Accept(_ context.Context, appealID int64, _, _ string) error {
	f.acceptCalls = append(f.acceptCalls, appealID)
	return nil
}

func (f *fakeAppealResolver) Reject(_ context.Context, appealID int64, _, _ string) error {
	f.rejectCalls = append(f.rejectCalls, appealID)
	return nil
}

func appealCallbackQuery(data string) *tbapi.CallbackQuery {
	return &tbapi.CallbackQuery{
		ID:      "cb1",
		From:    &tbapi.User{ID: 1, UserName: "admin"},
		Data:    data,
		Message: &tbapi.Message{MessageID: 50, Chat: tbapi.Chat{ID: 12345}, Text: "Апелляция"},
	}
}

func TestAdmin_callbackAppealResolve_Accept(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealNew}}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AA88"))
	require.NoError(t, err)
	assert.Equal(t, []int64{88}, resolver.acceptCalls)
}

func TestAdmin_callbackAppealResolve_Reject(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealNew}}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AR88"))
	require.NoError(t, err)
	assert.Equal(t, []int64{88}, resolver.rejectCalls)
}

func TestAdmin_callbackAppealResolve_AlreadyResolved(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealAccepted}}
	var answered string
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			if cb, ok := c.(tbapi.CallbackConfig); ok {
				answered = cb.Text
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AA88"))
	require.NoError(t, err)
	assert.Empty(t, resolver.acceptCalls, "already-resolved appeal is not re-accepted")
	assert.Equal(t, "Апелляция уже рассмотрена", answered)
}

func TestAdmin_callbackAppealResolve_BadID(t *testing.T) {
	resolver := &fakeAppealResolver{appealErr: errors.New("unused")}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AAxyz"))
	require.Error(t, err)
}
