# Подключение Inara

У Inara есть две отдельные настройки:

| Что | Для чего | Где выполняется |
| --- | --- | --- |
| Личный API-ключ | Определяет ваш профиль командира | [Настройки API вашего аккаунта Inara](https://inara.cz/elite/cmdr-settings-api/) |
| Разрешение имени приложения | Позволяет программе с именем `ED Copilot` отправлять данные в API | Разовый запрос автору Inara — [Artie](https://inara.cz/elite/cmdr/1/) |

Если вы уже вставили и сохранили личный ключ в ED Copilot, первый шаг готов. Создавать ещё один ключ не нужно. Успешный вход Frontier и его Client ID не заменяют ни один из этих шагов.

## Что сделать на сайте Inara

1. Войдите в свой аккаунт на Inara.
2. Откройте [профиль Artie](https://inara.cz/elite/cmdr/1/) — именно туда ведёт ссылка `let me know` в [официальном руководстве API](https://inara.cz/elite/inara-api-devguide/).
3. Отправьте Artie личное сообщение с запросом разрешить имя `ED Copilot` для API. В приложении есть кнопка **«Скопировать запрос для Inara»**, которая подготовит текст с текущей версией приложения. Проверьте его перед отправкой. Кнопка сама ничего на сайт не отправляет.
4. Дождитесь ответа, что имя приложения разрешено.
5. Вернитесь в ED Copilot → **«Синхронизация»**. Выберите **«Отправляет ED Copilot»**, включите Inara, отметьте **«Я получил от Inara подтверждение: имя ED Copilot разрешено для API»** и сохраните настройки.
6. Начните текущий сеанс Elite Dangerous Live и создайте новое поддерживаемое событие. Проверьте время **«Успешная отправка»** и ответ сервиса. Можно нажать **«Отправить очередь сейчас»**.

Галочка в приложении только сохраняет ваше подтверждение. Она не отправляет заявку, не регистрирует приложение на Inara и не выдаёт удалённое разрешение. Надпись «Ключ сохранён» означает только сохранение ключа на вашем компьютере.

Настройки API доступны после входа. Если ссылка открыла поиск командиров, сначала войдите в свой аккаунт и откройте её снова. Если личный ключ уже сохранён, этот экран для повторного создания ключа не нужен.

## Заготовка запроса

Ниже указаны четыре сведения, которые просит официальный Dev guide: точное имя, назначение, краткое описание и домашняя страница. Версию при копировании из приложения подставляет ED Copilot. В заготовку не входят ваш API-ключ, Client ID Frontier, пароль или данные аккаунта.

```text
Hello Artie,

I would like to register ED Copilot for development/testing access to the Inara API.

Exact application name sent in header.appName: ED Copilot
Homepage/source: https://github.com/Naenier/ED-Copilot

Purpose: A Windows desktop companion I use for my own Elite Dangerous Live commander. It reads local game journals and optional Frontier CAPI snapshots. The Inara integration updates supported commander-profile information: credits, ships/loadouts, cargo/materials, ranks, missions and travel events. It does not request market, station or star-system datasets from Inara.

Short description: Russian-language Elite Dangerous desktop companion with an activity-based overlay, trading/mining tools, engineering plans, fleet-carrier planning and optional commander-profile export to Inara.

The integration is in development and sends isBeingDeveloped=true. It uses batched requests, retries with backoff, Live-only checks, journal replay exclusion and event deduplication. Automated protocol checks use synthetic data and mocked responses; live-account export still needs validation after application approval. Only sources are published so far, without binary releases.

Please let me know if you need more information or changes before approving the application name. Thank you.
```

## Пока разрешения нет: отправка через EDMC

EDMC уже указан на [официальной странице поддерживаемых приложений Inara](https://inara.cz/elite/inara-api/).

1. В Copilot выберите **«Отправляет EDMC»** и сохраните настройки.
2. В самом EDMC настройте отправку в Inara с вашим личным API-ключом.
3. Если нужен статус EDMC внутри Copilot, установите локальный мост и выполните сопряжение по инструкции в разделе синхронизации.

В этом режиме внешнюю отправку выполняет EDMC под собственной идентификацией приложения. Copilot не выдаёт себя за EDMC и не отправляет те же события повторно. Отдельное разрешение имени `ED Copilot` для работы такого режима не требуется.

## Границы подключения

Inara получает поддерживаемые сведения вашего профиля. Эта интеграция не запрашивает у Inara общегалактическую базу рынков, станций или систем: её API таких наборов не предоставляет. Поиск рынков в Copilot использует другие указанные в приложении источники.

Флаг `isBeingDeveloped=true` помечает разработку и ограничивает глобальные события на стороне Inara. Он не заменяет разрешение имени приложения. Личные события могут обновлять профиль командира после получения доступа.

Проверено 7 ноября 2026 года: [Dev guide](https://inara.cz/elite/inara-api-devguide/), [описание заголовка API](https://inara.cz/elite/inara-api-docs/), [список приложений и ссылка на настройки ключа](https://inara.cz/elite/inara-api/). Запрос администратору от имени пользователя автоматически не отправлялся.
