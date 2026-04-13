# Этап 3. Multi-tenant core

Текущее состояние: код уже использует `InstanceID` и `gid` для логического разделения части данных, но это ещё не полноценный tenant model. Для SaaS нужно перевести все доменные сущности на явный `tenant_id`, а `gid` оставить только как временный миграционный мост.

## Задачи

- [ ] Спроектировать таблицы `tenants`, `tenant_chats`, `tenant_memberships`, `tenant_limits`, `tenant_statuses`.
- [ ] Добавить `tenant_id` в `rule_sets`, `reports`, `detected_spam`, `samples`, `dictionary`, `approved_users`, `locator`, `incoming_events`, `moderation_actions`, `audit records`.
- [ ] Реализовать миграцию существующего single-tenant инстанса в первый tenant с сохранением текущих данных и ссылок.
- [ ] Протащить `tenant_id` через все доменные контракты pipeline, control plane и web API.
- [ ] Заменить выборку по `gid` на выборку по `tenant_id` во всех storage-репозиториях и оставить `gid` только в миграционном слое.
- [ ] Добавить tenant-aware cache keys для `RuleSet`, policy profile, quotas и reputation data.
- [ ] Ввести per-tenant quotas для message throughput, report volume, LLM budget и retention windows.
- [ ] Ввести federation hooks для shared bans и inherited policies, но не включать их по умолчанию.
- [ ] Реализовать soft-delete tenant-а и безопасный offboarding с отзывом доступов и очисткой runtime cache.
- [ ] Добавить API-level rate limits и authz checks с tenant scope для всех control plane операций.
- [ ] Написать isolation integration tests, которые проверяют отсутствие утечки правил, audit, санкций и usage counters между tenant-ами.
- [ ] Добавить аудит SQL-запросов и repository tests на обязательный `tenant_id` filter для каждой выборки.

## Критерий завершения

- [ ] Два tenant-а могут параллельно модерировать разные чаты без пересечения конфигов и историй.
- [ ] Во всех mutable и queryable сущностях есть явный `tenant_id`.
- [ ] Тесты изоляции падают при любой выборке без tenant scope.
