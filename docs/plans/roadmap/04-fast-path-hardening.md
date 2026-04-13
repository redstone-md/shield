# Этап 4. Fast Path production-grade

Текущее состояние: в `lib/tgspam` уже есть сильный набор эвристик, classifier, duplicate detection, meta checks, CAS, Lua plugins и LLM hooks. Этот этап не добавляет “ещё один детектор”, а собирает текущие проверки в управляемый fast path с единым scoring и explainability payload.

## Задачи

- [ ] Вынести нормализацию в отдельный pipeline-модуль и добавить этапы cleanup для mixed scripts, zero-width, confusables и emoji noise.
- [ ] Перевести существующие проверки `classifier`, `stop words`, `meta`, `duplicates`, `abnormal spacing`, `plugins`, `CAS` на единый контракт `RiskSignal`.
- [ ] Добавить weighted aggregation, которая собирает итоговый `RiskScore` из отдельных сигналов без хардкода в `app/bot/spam.go`.
- [ ] Добавить explainability payload с matched rule IDs, signal weights, normalized text fragment и human-readable reason.
- [ ] Вынести rate-based и repeated payload эвристики в отдельное stateful хранилище, чтобы они не зависели от случайных кусков listener state.
- [ ] Добавить heuristics по профилю пользователя и канала: suspicious username, sender type, recent account behavior, repeated mentions.
- [ ] Разделить deterministic fast checks и probabilistic checks, чтобы policy мог принимать решение по-разному для explainable и ambiguous сигналов.
- [ ] Подготовить OCR-ready interface и text-extraction hooks без включения full vision path на этом этапе.
- [ ] Добавить replay harness на anonymized spam cases и измерять false positive / false negative по классам сигналов.
- [ ] Добавить benchmark suite для fast path latency и нагрузочных burst-сценариев.
- [ ] Подключить vector similarity только после стабилизации scoring и метрик, как отдельный optional detector под feature flag.
- [ ] Обновить docs и ADR по составу fast path и explainability contract.

## Критерий завершения

- [ ] Большинство типового спама обрабатывается fast path без участия LLM.
- [ ] Для каждого fast-path решения можно показать score, сработавшие сигналы и их вес.
- [ ] Есть replay и benchmark контур, который позволяет безопасно менять детекторы.
