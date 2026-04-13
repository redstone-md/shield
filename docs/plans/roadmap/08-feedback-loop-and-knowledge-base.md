# Этап 8. Feedback loop и knowledge base

Текущее состояние: система уже умеет обновлять spam/ham samples, принимать `/spam`, `/ban`, `/report` и вести `detected_spam`. Этот этап превращает разрозненные обновления в формальный pipeline знаний с ревью, версиями и безопасным rollout.

## Задачи

- [ ] Спроектировать label model для `confirmed_spam`, `false_positive`, `missed_spam`, `policy_override`, `report_abuse`.
- [ ] Перевести текущие ручные обновления spam/ham samples и admin actions в единый поток labels, а не в прямую запись в dataset.
- [ ] Создать knowledge base versioning для regex rules, stop phrases, ignored tokens, vector patterns и policy hints.
- [ ] Построить генератор кандидатов из incident data и labels для новых regex, stop phrases, repeated payload patterns и vector exemplars.
- [ ] Добавить review workflow для публикации кандидатов с обязательным approve/reject и записью автора решения.
- [ ] Реализовать безопасный publish flow для knowledge updates с rollback на предыдущую версию.
- [ ] Добавить shadow rollout и A/B rollout новых detectors или rule packs на части tenant-ов.
- [ ] Разделить tenant-local knowledge и global knowledge с явными правилами наследования и override.
- [ ] Научить control plane показывать diff между версиями knowledge base и impact preview на replay dataset.
- [ ] Добавить offline evaluation pipeline на anonymized incidents перед публикацией новой версии knowledge base.
- [ ] Добавить тесты и runbooks для отката неудачного knowledge update.
- [ ] Обновить документацию по lifecycle обучающих данных и безопасной публикации новых правил.

## Критерий завершения

- [ ] Новые паттерны попадают в fast path через управляемый knowledge workflow, а не через несвязанные ручные правки.
- [ ] Любой rollout knowledge update можно откатить до предыдущей версии.
- [ ] Tenant-local и global knowledge не смешиваются неявно.
