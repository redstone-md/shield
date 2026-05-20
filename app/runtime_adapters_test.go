package main

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redstone-md/shield/app/feedback"
)

func TestAutoLearnerAdapter_LearnSpam(t *testing.T) {
	t.Run("adds spam sample and generates candidates", func(t *testing.T) {
		sa := &sampleAdderSpy{}
		rs := &reviewGeneratorSpy{}
		a := autoLearnerAdapter{samples: sa, reviewSvc: rs}
		a.LearnSpam(context.Background(), "buy crypto cheap", "admin")

		assert.Len(t, sa.spamCalls, 1)
		assert.Equal(t, "buy crypto cheap", sa.spamCalls[0])
		assert.Len(t, rs.generateCalls, 1)
		assert.Equal(t, "buy crypto cheap", rs.generateCalls[0])
	})

	t.Run("skips empty text", func(t *testing.T) {
		sa := &sampleAdderSpy{}
		rs := &reviewGeneratorSpy{}
		a := autoLearnerAdapter{samples: sa, reviewSvc: rs}
		a.LearnSpam(context.Background(), "", "admin")

		assert.Empty(t, sa.spamCalls)
		assert.Empty(t, rs.generateCalls)
	})

	t.Run("nil samples still generates candidates", func(t *testing.T) {
		rs := &reviewGeneratorSpy{}
		a := autoLearnerAdapter{reviewSvc: rs}
		a.LearnSpam(context.Background(), "spam text", "admin")

		assert.Len(t, rs.generateCalls, 1)
	})

	t.Run("nil reviewSvc still adds samples", func(t *testing.T) {
		sa := &sampleAdderSpy{}
		a := autoLearnerAdapter{samples: sa}
		a.LearnSpam(context.Background(), "spam text", "admin")

		assert.Len(t, sa.spamCalls, 1)
	})
}

func TestAutoLearnerAdapter_LearnHam(t *testing.T) {
	t.Run("adds ham sample", func(t *testing.T) {
		sa := &sampleAdderSpy{}
		a := autoLearnerAdapter{samples: sa}
		a.LearnHam(context.Background(), "normal message", "admin")

		assert.Len(t, sa.hamCalls, 1)
		assert.Equal(t, "normal message", sa.hamCalls[0])
	})

	t.Run("skips empty text", func(t *testing.T) {
		sa := &sampleAdderSpy{}
		a := autoLearnerAdapter{samples: sa}
		a.LearnHam(context.Background(), "", "admin")

		assert.Empty(t, sa.hamCalls)
	})
}

type sampleAdderSpy struct {
	mu        sync.Mutex
	spamCalls []string
	hamCalls  []string
}

func (s *sampleAdderSpy) AddSpamSample(_ context.Context, text string) error {
	s.mu.Lock()
	s.spamCalls = append(s.spamCalls, text)
	s.mu.Unlock()
	return nil
}

func (s *sampleAdderSpy) AddHamSample(_ context.Context, text string) error {
	s.mu.Lock()
	s.hamCalls = append(s.hamCalls, text)
	s.mu.Unlock()
	return nil
}

type reviewGeneratorSpy struct {
	mu            sync.Mutex
	generateCalls []string
}

func (r *reviewGeneratorSpy) GenerateFromSpamText(_ context.Context, _ int64, text string) ([]feedback.CandidateEntry, error) {
	r.mu.Lock()
	r.generateCalls = append(r.generateCalls, text)
	r.mu.Unlock()
	return nil, nil
}
