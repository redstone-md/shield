# Этап 6. Slow Path для LLM и Vision

Текущее состояние: OpenAI и Gemini уже встроены в текущий detection flow, а user reports частично умеют форсировать LLM review. Для SaaS это нужно перевести из inline-поведения в управляемый slow path с очередями, бюджетами и воспроизводимостью.

## Задачи

- [ ] Вынести текущие OpenAI/Gemini проверки из основного fast path в отдельный `slowpath` worker и оставить синхронный режим только как временный feature flag.
- [ ] Разделить internal queue на `fast` и `slow` очереди и формализовать причины эскалации в slow path.
- [ ] Добавить контракт `SlowPathRequest` с полями `tenant_id`, `event_id`, `reason`, `prompt_version`, `budget_class`, `content_refs`.
- [ ] Перевести текущий report-triggered LLM review на публикацию `SlowPathRequest`, а не на прямой вызов detector внутри `userReports`.
- [ ] Создать prompt/version registry в БД и перестать хранить рабочие промпты только в env-параметрах.
- [ ] Сохранять для каждого LLM/Vision вызова модель, provider, prompt version, latency, token usage, cost estimate и итоговую structured response.
- [ ] Добавить per-tenant budget enforcement и fallback политику при исчерпании бюджета или деградации провайдера.
- [ ] Добавить circuit breaker и retry policy для OpenAI, Gemini и будущих vision providers.
- [ ] Спроектировать `VisionProvider` и `OCRProvider` интерфейсы и подключить минимум один OCR путь для изображений без текста в Telegram caption.
- [ ] Объединить slow-path output с fast-path signals в единый `DetectionResult`, который понимает policy engine.
- [ ] Добавить integration-тесты на escalation flow `fast -> slow -> policy -> action`.
- [ ] Обновить документацию по модели воспроизводимости решений и затратам slow path.

## Критерий завершения

- [ ] LLM и Vision не вызываются на каждое сообщение, а только по явным причинам эскалации.
- [ ] По каждому slow-path решению видно модель, prompt version, входной контекст, latency и budget impact.
- [ ] Отключение slow path не ломает fast path и policy execution.
