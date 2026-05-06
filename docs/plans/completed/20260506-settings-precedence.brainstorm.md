# Settings Precedence Brainstorm

## Problem

Runtime rule sets are loaded from DB, but explicit operator configuration from CLI or environment must win over DB values. Applying every parsed option blindly is wrong because parser defaults would overwrite DB rule sets.

## Options

- Apply full `bootstrapRuleSet(opts)` over active DB rule set. Rejected: parser defaults overwrite persisted tuning.
- Override only `dry` and `soft_ban`. Rejected: user clarified the rule is broader: CLI/env > DB.
- Detect explicitly provided CLI/env values and overlay only those fields onto the active rule set. Chosen: preserves DB for unspecified values and honors operator overrides.

## Scope

In scope: rule-set-backed runtime fields assembled from `options` and live reload behavior.

Out of scope: non-rule-set process settings like token, DB URL, server listen address, and Telegram admin routing.
