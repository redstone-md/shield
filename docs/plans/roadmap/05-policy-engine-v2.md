# Этап 5. Policy Engine v2

Текущее состояние: decision logic сейчас распределена между `app/events`, `userReports`, moderation helper functions и runtime flags. Этот этап отделяет бизнес-решение от детекции и делает санкции конфигурируемыми на уровне tenant-а.

## Задачи

- [ ] Создать отдельный пакет `app/policy` с контрактами `PolicyInput`, `Decision`, `ActionPlan`, `DecisionExplanation`.
- [ ] Перенести текущие функции `spamPenalty`, report penalties, dry-run branching и moderation action text в `app/policy`.
- [ ] Ввести профили строгости `permissive`, `balanced`, `strict` как конфигурируемые policy presets.
- [ ] Определить матрицу действий по типу риска: `spam`, `abuse`, `scam`, `raid`, `nsfw`, `unknown`.
- [ ] Добавить escalation chain `warn -> mute -> delete+mute -> ban` с учётом strike history и роли пользователя в чате.
- [ ] Научить policy учитывать источник сигнала: deterministic fast path, slow path, user report, manual admin action.
- [ ] Добавить dry-run и shadow-decision режим для безопасного выката новых policy rules.
- [ ] Реализовать policy simulation на исторических incident records и встроить её в control plane.
- [ ] Вынести action selection из Telegram-specific слоя, чтобы policy не знал о конкретных API командах Telegram.
- [ ] Добавить versioning policy profiles и запись `policy_version` в audit trail.
- [ ] Добавить тесты на одинаковые сигналы при разных policy profiles, ролях и histories пользователя.
- [ ] Расширить control plane UI/API управлением policy profile и preview будущих санкций.

## Критерий завершения

- [ ] Один и тот же набор сигналов может приводить к разным действиям в зависимости от policy profile tenant-а.
- [ ] Detector возвращает сигналы, а не принимает бизнес-решения о бане сам.
- [ ] Новое policy правило можно включить через config/version switch без переписывания детекторов.
