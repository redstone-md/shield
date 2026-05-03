# Empty LLM Message Brainstorm

## Current State

- Image-only Telegram updates arrive with empty text and image metadata.
- Fast-path text checks still invoke OpenAI when short-message LLM checking is enabled.
- `go-openai` omits empty string content, and OpenAI-compatible vLLM rejects the resulting user message as `content: null` or missing.
- Slow-path vision runs after the fast path and is the correct place for image content analysis.

## Problem

Image-only messages should not send a text-only LLM request with no user text. The fast-path detector should skip text LLM providers when there is no text to analyze, while leaving vision slow-path behavior intact.

## Options

1. Send a placeholder string such as "image-only message" to the text LLM.
   - Pro: avoids invalid API payloads.
   - Con: asks a text model to judge absent content and can create noisy moderation decisions.
2. Skip text LLM checks for blank normalized messages.
   - Pro: keeps text and vision responsibilities separate, avoids bad payloads, preserves slow-path image analysis.
   - Con: deployments without slow-path vision rely on meta checks for image-only moderation.
3. Special-case only OpenAI request building to force `Content: " "`.
   - Pro: smallest payload-level change.
   - Con: leaves Gemini and future text LLMs with inconsistent behavior and still sends meaningless text.

## Direction

Pick option 2. Add a shared detector-level blank-text gate before provider calls. Cover it with a regression test that enables short-message OpenAI checking and sends an image-only request. Add a slow-path vision request-shape test so multimodal requests remain array content.

## Constraints And Risks

- Keep changes inside existing private detector and slowpath adapter boundaries.
- Do not change public API contracts or storage schema.
- Do not touch protected untracked files.
- Preserve files under 500 LOC and functions under 80 LOC.
