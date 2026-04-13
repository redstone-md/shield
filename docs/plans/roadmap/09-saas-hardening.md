# Этап 9. SaaS hardening и эксплуатационная зрелость

Текущее состояние: у проекта уже есть backup/export, Postgres support, Docker-compose сценарии и зрелый набор тестов. Для SaaS-фазы нужно довести это до tenant-aware operations, metering, security hardening и доказанного масштабирования.

## Задачи

- [ ] Добавить usage metering для входящих событий, slow-path вызовов, хранения истории и manual review нагрузки по каждому tenant-у.
- [ ] Спроектировать billing abstraction и feature gating для тарифов, квот и premium capabilities.
- [ ] Реализовать tenant onboarding/offboarding flows с автоматическим созданием базовых конфигов, ролей и runtime cache entries.
- [ ] Сделать tenant-aware backup/restore поверх текущих backup/export механизмов и проверить точечное восстановление одного tenant-а.
- [ ] Добавить SLO/SLA метрики и error budget для gateway, fast path, slow path, control plane и action executor.
- [ ] Вынести queue backend на внешний брокер только после доказанной нагрузки и сохранить in-memory backend как dev-mode.
- [ ] Подготовить горизонтальное масштабирование worker-ов и проверить partitioning по tenant-ам и типам очередей.
- [ ] Добавить secrets management, шифрование чувствительных данных и аудит административного доступа.
- [ ] Ввести retention и deletion policy для персональных данных, audit payloads и appeal materials.
- [ ] Добавить security review на multi-tenant authz, rate limits, injection vectors и recovery flows.
- [ ] Подготовить эксплуатационные runbooks: broker outage, Telegram API degradation, provider outage, cache corruption, tenant restore.
- [ ] Добавить нагрузочные и chaos-тесты на burst spam-атаки, backlog growth и восстановление после сбоя broker/cache/db.

## Критерий завершения

- [ ] Система обслуживает несколько tenant-ов с предсказуемой стоимостью и наблюдаемыми квотами.
- [ ] Есть runbooks, backup/restore и security controls уровня production SaaS.
- [ ] Решение о выносе домена в отдельный сервис принимается по измеримой нагрузке, а не заранее.
