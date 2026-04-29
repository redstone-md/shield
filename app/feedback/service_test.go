package feedback

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLabelStore struct {
	entries []LabelEntry
	nextID  int64
}

func (m *mockLabelStore) Create(_ context.Context, entry LabelEntry) (LabelEntry, error) {
	m.nextID++
	entry.ID = m.nextID
	m.entries = append(m.entries, entry)
	return entry, nil
}

func (m *mockLabelStore) GetByID(_ context.Context, id int64) (LabelEntry, error) {
	for _, e := range m.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return LabelEntry{}, ErrNotFound
}

func (m *mockLabelStore) GetByDetectedSpamID(_ context.Context, spamID int64) ([]LabelEntry, error) {
	var res []LabelEntry
	for _, e := range m.entries {
		if e.DetectedSpamID == spamID {
			res = append(res, e)
		}
	}
	return res, nil
}

func (m *mockLabelStore) GetByIncidentID(_ context.Context, incidentID int64) ([]LabelEntry, error) {
	var res []LabelEntry
	for _, e := range m.entries {
		if e.IncidentID == incidentID {
			res = append(res, e)
		}
	}
	return res, nil
}

func (m *mockLabelStore) List(_ context.Context, filter LabelFilter) ([]LabelEntry, error) {
	var res []LabelEntry
	for _, e := range m.entries {
		if filter.Label != "" && e.Label != filter.Label {
			continue
		}
		if filter.LabeledBy != "" && e.LabeledBy != filter.LabeledBy {
			continue
		}
		res = append(res, e)
	}
	return res, nil
}

func (m *mockLabelStore) Stats(_ context.Context) (map[Label]int, error) {
	result := make(map[Label]int)
	for _, e := range m.entries {
		result[e.Label]++
	}
	return result, nil
}

type mockSampleAdder struct {
	spamTexts []string
	hamTexts  []string
}

func (m *mockSampleAdder) AddSpamSample(_ context.Context, text string) error {
	m.spamTexts = append(m.spamTexts, text)
	return nil
}

func (m *mockSampleAdder) AddHamSample(_ context.Context, text string) error {
	m.hamTexts = append(m.hamTexts, text)
	return nil
}

type mockSpamTextProvider struct {
	texts map[int64]string
}

func (m *mockSpamTextProvider) GetSpamText(_ context.Context, spamID int64) (string, error) {
	text, ok := m.texts[spamID]
	if !ok {
		return "", ErrNotFound
	}
	return text, nil
}

func TestService_Label_ConfirmedSpam(t *testing.T) {
	store := &mockLabelStore{}
	samples := &mockSampleAdder{}
	spamTxt := &mockSpamTextProvider{texts: map[int64]string{42: "buy viagra now"}}
	svc := NewService(store, samples, spamTxt)

	created, err := svc.Label(t.Context(), LabelEntry{
		DetectedSpamID: 42,
		Label:          LabelConfirmedSpam,
		LabeledBy:      "admin",
	})
	require.NoError(t, err)
	assert.True(t, created.ID > 0)
	assert.Equal(t, LabelConfirmedSpam, created.Label)
	assert.Equal(t, []string{"buy viagra now"}, samples.spamTexts)
	assert.Empty(t, samples.hamTexts)
}

func TestService_Label_FalsePositive(t *testing.T) {
	store := &mockLabelStore{}
	samples := &mockSampleAdder{}
	spamTxt := &mockSpamTextProvider{texts: map[int64]string{10: "hello world"}}
	svc := NewService(store, samples, spamTxt)

	_, err := svc.Label(t.Context(), LabelEntry{
		DetectedSpamID: 10,
		Label:          LabelFalsePositive,
		LabeledBy:      "user1",
	})
	require.NoError(t, err)
	assert.Empty(t, samples.spamTexts)
	assert.Equal(t, []string{"hello world"}, samples.hamTexts)
}

func TestService_Label_NoSideEffectWithoutSpamID(t *testing.T) {
	store := &mockLabelStore{}
	samples := &mockSampleAdder{}
	spamTxt := &mockSpamTextProvider{}
	svc := NewService(store, samples, spamTxt)

	_, err := svc.Label(t.Context(), LabelEntry{
		IncidentID: 99,
		Label:      LabelPolicyOverride,
		LabeledBy:  "admin",
	})
	require.NoError(t, err)
	assert.Empty(t, samples.spamTexts)
	assert.Empty(t, samples.hamTexts)
}

func TestService_Stats(t *testing.T) {
	store := &mockLabelStore{}
	svc := NewService(store, nil, nil)

	_, _ = svc.Label(t.Context(), LabelEntry{Label: LabelConfirmedSpam})
	_, _ = svc.Label(t.Context(), LabelEntry{Label: LabelConfirmedSpam})
	_, _ = svc.Label(t.Context(), LabelEntry{Label: LabelFalsePositive})

	stats, err := svc.Stats(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Confirmed)
	assert.Equal(t, 1, stats.FalsePositive)
	assert.Equal(t, 3, stats.Total)
}

func TestService_List(t *testing.T) {
	store := &mockLabelStore{}
	svc := NewService(store, nil, nil)

	_, _ = svc.Label(t.Context(), LabelEntry{Label: LabelConfirmedSpam, LabeledBy: "a"})
	_, _ = svc.Label(t.Context(), LabelEntry{Label: LabelFalsePositive, LabeledBy: "b"})

	spamOnly, err := svc.List(t.Context(), LabelFilter{Label: LabelConfirmedSpam})
	require.NoError(t, err)
	assert.Len(t, spamOnly, 1)

	byUser, err := svc.List(t.Context(), LabelFilter{LabeledBy: "b"})
	require.NoError(t, err)
	assert.Len(t, byUser, 1)
	assert.Equal(t, LabelFalsePositive, byUser[0].Label)
}

func TestService_GetByDetectedSpamID(t *testing.T) {
	store := &mockLabelStore{}
	svc := NewService(store, nil, nil)

	_, _ = svc.Label(t.Context(), LabelEntry{DetectedSpamID: 42, Label: LabelConfirmedSpam})
	_, _ = svc.Label(t.Context(), LabelEntry{DetectedSpamID: 42, Label: LabelFalsePositive})

	labels, err := svc.GetByDetectedSpamID(t.Context(), 42)
	require.NoError(t, err)
	assert.Len(t, labels, 2)
}
