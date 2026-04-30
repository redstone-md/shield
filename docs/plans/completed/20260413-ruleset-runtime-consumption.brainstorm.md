# 2026-04-13 RuleSet Runtime Consumption Brainstorm

## Problem

The repo persists a single-tenant `RuleSet`, but the runtime still builds detector behavior and moderation/report settings from scattered `opts` values. That means persisted active rules are not yet the source of truth for live decisions.

## Constraints

- Keep credentials and low-level provider wiring in runtime options for now.
- Move actual moderation thresholds and feature flags to the persisted `RuleSet`.
- Prove that an active database snapshot overrides bootstrap defaults.

## Decision

Load the active `RuleSet` during runtime assembly, build the detector from that snapshot, and wire listener moderation/report behavior from the same loaded rule set.

## Verification Target

- detector duplicate/meta/abnormal-spacing/LLM rule flags come from the active `RuleSet`
- listener moderation/report config comes from the active `RuleSet`
- a persisted active version overrides bootstrap defaults at startup
