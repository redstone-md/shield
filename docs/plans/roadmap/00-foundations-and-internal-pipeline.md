# Этап 0. Foundations и внутренний pipeline

Текущее состояние: в репозитории уже есть рабочий single-process контур `Telegram -> app/events -> app/bot -> lib/tgspam -> Telegram/WebUI`. Этот этап не строит новый продукт с нуля, а вынимает из текущего monolith доменные границы и внутренний асинхронный шов, на который потом можно безопасно наращивать control plane, multi-tenant и slow path.

## Задачи

- [x] Зафиксировать ADR с mapping текущих пакетов на bounded contexts: `controlplane`, `gateway`, `detection`, `policy`, `audit`.
- [x] Описать единый внутренний контракт `IncomingEvent`, `DetectionResult`, `PolicyDecision`, `ModerationActionResult` и положить его в отдельный пакет без Telegram-specific деталей.
- [x] Создать интерфейс `Queue` и in-memory реализацию на каналах, чтобы отделить ingestion от обработки без немедленного ввода RabbitMQ/NATS.
- [x] Перевести `app/events/listener.go` с прямого вызова обработки на публикацию `IncomingEvent` в internal queue.
- [x] Создать worker-процессор, который читает событие из queue и вызывает detection, policy и action слои через интерфейсы, а не через связанные напрямую структуры.
- [x] Вынести применение санкций из `app/events` в отдельный `action executor`, чтобы `events` отвечал только за Telegram ingestion и transport-specific адаптацию.
- [x] Вынести расчёт policy decision из `app/events` в отдельный пакет с минимальным правилом `allow/delete/restrict/ban`.
- [ ] Вынести запись результатов модерации в отдельный `audit writer`, который умеет сохранять входное событие, решение и результат исполнения.
- [ ] Добавить `event_id` и `correlation_id` в логирование всех шагов пайплайна и протащить их через `app/events`, `app/bot`, `app/storage`, `app/webapi`.
- [ ] Добавить readiness/health endpoints для основного runtime, а не только для web API.
- [ ] Собрать smoke-тест на tracer bullet: `Telegram update -> queue -> worker -> detection -> policy -> action -> audit`.
- [x] Обновить `docs/ROADMAP.md` или отдельный ADR ссылкой на новые доменные контракты и execution order.

## Критерий завершения

- [ ] `app/main.go` собирает runtime из доменных интерфейсов, а не из одной связки прямых вызовов.
- [ ] Один и тот же spam case проходит end-to-end через internal queue без изменения пользовательского поведения.
- [ ] В логах и тестах видно `event_id` и полный путь обработки события.
