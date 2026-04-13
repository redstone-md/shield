# Этап 1. Single-tenant rules, idempotency и безопасное исполнение

Текущее состояние: single-tenant антиспам уже работает, но правила в основном размазаны между env-конфигом, `app/events`, `app/bot` и `lib/tgspam`, а входящие Telegram updates не оформлены как идемпотентные доменные события. Этот этап превращает существующее поведение в явный single-tenant moderation core.

## Задачи

- [ ] Ввести persistent `RuleSet` для одного workspace и хранить его в БД как отдельную сущность, а env-параметры использовать только как bootstrap defaults.
- [ ] Создать миграцию для таблиц `rule_sets`, `rule_set_versions`, `incoming_events`, `moderation_actions`.
- [ ] Реализовать загрузчик `RuleSet`, который конвертирует текущие флаги `meta`, `duplicates`, `space`, `moderation`, `report`, `openai`, `gemini` в доменную конфигурацию.
- [ ] Добавить idempotency key для Telegram события на базе `update_id`, `chat_id`, `message_id`, `edited_message_id` и сохранять его в `incoming_events`.
- [ ] Научить pipeline повторно получать уже обработанное решение по idempotency key вместо повторного бана или удаления.
- [ ] Вынести нормализацию текста из детекторов в отдельный модуль с этапами lower-case, trim, cleanup zero-width, canonical whitespace и script folding hooks.
- [ ] Перевести текущие проверки из `lib/tgspam` и `app/bot/spam.go` на чтение из `RuleSet`, а не из разрозненных опций runtime.
- [ ] Создать `ActionExecutor` с явными командами `DeleteMessage`, `MuteUser`, `BanUser`, `BanSenderChat`, `WarnUser`.
- [ ] Добавить журнал исполнения action-команд с retry state, last error и idempotent replay.
- [ ] Перенести strike escalation и report-based penalties на использование общего `ActionExecutor`, а не разрозненных вызовов из `events`.
- [ ] Расширить audit-запись так, чтобы для каждого решения сохранялись `signal source`, `score`, `matched rules` и `rule_set_version`.
- [ ] Добавить integration-тесты на повторную доставку одного update, повторное выполнение action и восстановление после ошибки Telegram API.

## Критерий завершения

- [ ] Single-tenant чат продолжает модерироваться без регрессий, но все решения строятся из `RuleSet`.
- [ ] Повторный Telegram retry не приводит к двойному бану, двойному delete или двойной записи в audit.
- [ ] Любое решение можно связать с конкретной версией `RuleSet` и idempotency key.
