# Этап 2. Control Plane MVP поверх текущего web UI/API

Текущее состояние: `app/webapi` уже умеет показывать настройки, работать с sample data, approved users, dictionary и detected spam. Для roadmap выгоднее расширять этот слой до control plane, чем параллельно строить новый UI с нуля. Telegram Mini App можно добавлять как тонкую оболочку после стабилизации API.

## Задачи

- [ ] Создать отдельный пакет `app/controlplane` с сервисами управления правилами, чатами, ролями и runtime cache invalidation.
- [ ] Добавить CRUD API для `RuleSet`, whitelist, blacklist, moderation profile и report settings в `app/webapi`.
- [ ] Перевести текущую страницу settings из read-only режима в control plane экран с сохранением изменений через новые API.
- [ ] Добавить хранение `chat`, `workspace`, `admin membership` и `role` в БД, даже если на этом этапе обслуживается только один workspace.
- [ ] Ввести роли `owner`, `admin`, `viewer` и авторизацию на уровне control plane endpoints.
- [ ] Вынести доступ к runtime-конфигу через интерфейс cache store и реализовать сначала in-process cache, затем Redis-адаптер под тем же контрактом.
- [ ] Добавить инвалидацию cache после изменения `RuleSet`, словарей, approved users и moderation profile.
- [ ] Сформировать read model для последних инцидентов и последних действий модерации, чтобы control plane не читал сырые operational tables напрямую.
- [ ] Перенести управление approved users и dictionary под единые control plane use cases, а не под прямые вызовы detector/storage из web handlers.
- [ ] Подготовить auth contract для Telegram Mini App или Telegram Login так, чтобы его можно было подключить без переписывания доменной логики.
- [ ] Добавить acceptance-тесты на обновление правил без рестарта воркера и на почти мгновенное применение обновлений в runtime.
- [ ] Обновить документацию по admin flows и runtime config lifecycle.

## Критерий завершения

- [ ] Администратор меняет правила через web API/UI без редактирования env и без перезапуска процесса.
- [ ] Worker получает новую конфигурацию через cache refresh и применяет её к новым событиям.
- [ ] `app/webapi` больше не ходит напрямую в detector для настроек, а работает через `controlplane` сервисы.
