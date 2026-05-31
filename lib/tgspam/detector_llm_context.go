package tgspam

import (
	"strings"

	"github.com/redstone-md/shield/lib/spamcheck"
)

func shouldSkipTextLLM(msg string) bool {
	return strings.TrimSpace(msg) == ""
}

func (d *Detector) llmContextForRequest(req spamcheck.Request) llmContext {
	ctx := llmContext{
		RequestContext: req.LLMContext,
	}
	if d.LLMHistoryContextSize > 0 {
		ctx.RecentChatMessages = d.llmHistory.Last(d.LLMHistoryContextSize)
	}
	return ctx
}

func (d *Detector) addToLLMHistory(req spamcheck.Request) {
	if req.CheckOnly || req.Msg == "" {
		return
	}

	d.llmHistory.Push(req)
	if req.UserID == "" {
		return
	}

	h, ok := d.userHistory[req.UserID]
	if !ok {
		h = spamcheck.NewLastRequests(llmUserContextSize)
		d.userHistory[req.UserID] = h
	}
	h.Push(req)
}

func (d *Detector) applyLLMConsensus(baseSpam bool, results []detectorLLMResult, mode LLMConsensusMode) bool {
	if len(results) == 0 {
		return baseSpam
	}

	switch d.normalizeLLMConsensusMode(mode) {
	case LLMConsensusAll:
		for _, result := range results {
			if !result.flip {
				return baseSpam
			}
		}
		return !baseSpam
	default:
		for _, result := range results {
			if result.flip {
				return !baseSpam
			}
		}
		return baseSpam
	}
}
