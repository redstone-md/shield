# Этап 7. Audit, review и appeals

Текущее состояние: в проекте уже есть `detected_spam`, `reports`, admin chat flows и web UI для просмотра части данных, но это ещё не полный incident management. Этот этап собирает все события модерации в разборчивый operational контур.

## Задачи

- [ ] Спроектировать incident model с сущностями `incident`, `incident_signal`, `incident_decision`, `incident_action`, `incident_comment`, `appeal`.
- [ ] Научить pipeline писать полный audit trail для каждого события: raw input, normalized payload, signals, policy decision, action result, final status.
- [ ] Связать текущие `reports`, `detected_spam`, admin actions и automatic moderation в единый incident timeline.
- [ ] Добавить read model для списка инцидентов, карточки инцидента и истории действий модераторов.
- [ ] Расширить `app/webapi` страницами incident list, incident detail, review queue и appeal detail вместо создания нового отдельного admin backend.
- [ ] Реализовать ручной review flow для user reports и ambiguous slow-path cases.
- [ ] Добавить replay endpoint, который повторно прогоняет сохранённый payload через текущий fast path, slow path gate и policy engine без side effects.
- [ ] Добавить reason taxonomy и обязать policy/detection писать structured reason code, а не только текстовое описание.
- [ ] Реализовать appeal workflow со статусами `new`, `triaged`, `accepted`, `rejected`, `replayed`, `escalated`.
- [ ] Добавить redaction и retention policy для хранения чувствительных данных в incident payload.
- [ ] Добавить integration-тесты на разбор полного кейса без обращения к продовым логам.
- [ ] Обновить runbooks для расследования false positive и спорных банов.

## Критерий завершения

- [ ] Любое решение модерации можно разобрать в UI/API по одному incident ID.
- [ ] Есть управляемый manual review и appeals контур, не завязанный на чтение сырых логов.
- [ ] Replay на тех же входных данных воспроизводит все шаги решения без реального исполнения санкций.
