package bot

import (
	"context"
	"errors"
	"github.com/redstone-md/shield/app/bot/mocks"
	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSpamFilter_DynamicSamples(t *testing.T) {
	tests := []struct {
		name        string
		spamSamples []string
		hamSamples  []string
		readErr     error
		expectError bool
	}{
		{
			name:        "successful read",
			spamSamples: []string{"spam1", "spam2"},
			hamSamples:  []string{"ham1", "ham2"},
		},
		{
			name:        "read error",
			readErr:     errors.New("read error"),
			expectError: true,
		},
		{
			name:        "empty response",
			spamSamples: []string{},
			hamSamples:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			samplesStore := &mocks.SamplesStoreMock{
				ReadFunc: func(ctx context.Context, t storage.SampleType, o storage.SampleOrigin) ([]string, error) {
					if tc.readErr != nil {
						return nil, tc.readErr
					}
					if t == storage.SampleTypeSpam {
						return tc.spamSamples, nil
					}
					return tc.hamSamples, nil
				},
			}

			s := NewSpamFilterWithRoles(nil, nil, nil, nil, SpamConfig{
				SamplesStore: samplesStore,
			})

			spam, ham, err := s.DynamicSamples(context.Background())
			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.spamSamples, spam)
			assert.Equal(t, tc.hamSamples, ham)

			calls := samplesStore.ReadCalls()
			require.Len(t, calls, 2)
			assert.Equal(t, storage.SampleTypeSpam, calls[0].T)
			assert.Equal(t, storage.SampleTypeHam, calls[1].T)
			assert.Equal(t, storage.SampleOriginUser, calls[0].O)
			assert.Equal(t, storage.SampleOriginUser, calls[1].O)
		})
	}
}

func TestSpamFilter_RemoveDynamicSamples(t *testing.T) {
	tests := []struct {
		name        string
		sample      string
		sampleType  string // "spam" or "ham"
		deleteErr   error
		expectError bool
	}{
		{
			name:       "remove spam success",
			sample:     "spam sample",
			sampleType: "spam",
		},
		{
			name:        "remove spam delete error",
			sample:      "spam sample",
			sampleType:  "spam",
			deleteErr:   errors.New("delete error"),
			expectError: true,
		},
		{
			name:       "remove ham success",
			sample:     "ham sample",
			sampleType: "ham",
		},
		{
			name:        "remove ham delete error",
			sample:      "ham sample",
			sampleType:  "ham",
			deleteErr:   errors.New("delete error"),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := &mocks.SampleUpdaterMock{
				RemoveHamFunc: func(msg string) error {
					assert.Equal(t, tc.sample, msg)
					return tc.deleteErr
				},
				RemoveSpamFunc: func(msg string) error {
					assert.Equal(t, tc.sample, msg)
					return tc.deleteErr
				},
			}

			samplesStore := &mocks.SamplesStoreMock{}

			dictStore := &mocks.DictStoreMock{}

			s := NewSpamFilterWithRoles(nil, nil, updater, nil, SpamConfig{
				SamplesStore: samplesStore,
				DictStore:    dictStore,
				TenantID:     "gr1",
			})

			var err error
			switch tc.sampleType {
			case "spam":
				err = s.RemoveDynamicSpamSample(tc.sample)
			case "ham":
				err = s.RemoveDynamicHamSample(tc.sample)
			}

			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.sampleType == "spam" {
				assert.Len(t, updater.RemoveSpamCalls(), 1)
				assert.Equal(t, tc.sample, updater.RemoveSpamCalls()[0].Msg)
			}
			if tc.sampleType == "ham" {
				assert.Len(t, updater.RemoveHamCalls(), 1)
				assert.Equal(t, tc.sample, updater.RemoveHamCalls()[0].Msg)
			}
		})
	}
}

func TestSpamFilter_ApplyRuleSet(t *testing.T) {
	sf := NewSpamFilterWithRoles(nil, nil, nil, nil, SpamConfig{Dry: false})
	assert.False(t, sf.params.Dry)

	sf.ApplyRuleSet(rules.RuleSet{
		Moderation: rules.ModerationRules{DryRun: true},
	})
	assert.True(t, sf.params.Dry, "dry mode should be updated from rule set")

	sf.ApplyRuleSet(rules.RuleSet{
		Moderation: rules.ModerationRules{DryRun: false},
	})
	assert.False(t, sf.params.Dry, "dry mode should be toggled back")
}
