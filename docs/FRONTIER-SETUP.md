# Регистрация ED Copilot в Frontier Developer Zone

Client ID создаёт владелец приложения в своём аккаунте Frontier. Этот репозиторий не означает, что Frontier уже зарегистрировала или одобрила приложение.

## Форма Create Client

Откройте [Frontier Developer Zone](https://auth.frontierstore.net/client/signup) в браузере и войдите в свой аккаунт. Для показанной формы заполните:

| Поле | Значение |
| --- | --- |
| App Name | `ED Copilot` |
| Application Homepage URL | `https://github.com/Naenier/ED-Copilot` |
| Scopes — auth | Оставить включённым. Это базовый доступ к идентификатору и сведениям аккаунта. |
| Scopes — capi | Оставить включённым. Нужен доступ к сведениям командира через Companion API; требует auth. |
| Short Description | Текст ниже. |

```text
Personal desktop companion for Elite Dangerous: local journals, commander and ship status, trading, mining, engineering and fleet-carrier planning, with optional authorized Companion API access.
```

Прочитайте условия Frontier, затем отметьте `I agree to the terms and conditions` и нажмите `SUBMIT`, если согласны с ними. Рассмотрение заявки и доступные настройки определяет Frontier.

## После создания клиента

1. В Developer Zone откройте карточку клиента ED Copilot / `View` и скопируйте **Client ID**. Не используйте Client ID от EDMC или другого приложения.
2. В показанной карточке есть раздел **Endpoints** с полем `New endpoint` и требованием HTTPS. **Для выбранной настольной схемы входа ED Copilot оставьте этот список пустым. Ничего в это поле добавлять не требуется.**

   ED Copilot использует нативный OAuth-клиент с PKCE и передаёт локальный адрес возврата сам при начале входа:

   ```text
   http://localhost:7122/api/frontier/callback
   ```

   Это адрес только на вашем компьютере, на котором запущен ED Copilot. Текущая реализация EDMC также использует локальный HTTP-возврат для нативного PKCE-клиента, с портом, выбранным при запуске. На этом поведении основана выбранная схема ED Copilot. HTTPS-адрес сайта и нативный адрес возврата — разные настройки. `Application Homepage URL` остаётся ссылкой на GitHub.

   Не подменяйте локальный адрес на `https://localhost:...` или `https://127.0.0.1:...`: приложение не запускает HTTPS-сервер, поэтому такой возврат не сработает. Не указывайте GitHub в Endpoint: репозиторий не обрабатывает ответы авторизации.
3. В ED Copilot откройте раздел авторизации Frontier, вставьте Client ID, сохраните и выберите вход через Frontier.
4. Оставьте ED Copilot запущенным, войдите на настоящем сайте Frontier и разрешите доступ. Браузер вернётся на локальный адрес; приложение проверит ответ и сохранит зашифрованные токены.
5. Обновите сведения CAPI в приложении. Появление Client ID ещё не подтверждает успешную авторизацию: она проверяется после этого входа.

Текущая реализация использует публичный клиент OAuth 2.0 с **PKCE S256**, scopes `auth capi`. **Shared Key / Client Secret вводить не требуется.** Не отправляйте Shared Key, токены, полный адрес callback с параметрами `code` или файлы профиля в GitHub. Сам Client ID не является паролем.

Если приложение запущено на другом локальном порту, оно само передаёт соответствующий callback. Адрес можно посмотреть в разделе Frontier приложения; вручную менять HTTPS-список Endpoints для этой схемы не нужно.

Проверки кода выполняются с имитацией ответов Frontier. Анонимная проверка текущего Client ID дошла до страницы входа; она не заменяет полный вход и подтверждение доступа владельцем аккаунта. Если после входа Frontier сообщает об отклонённом адресе, сохраните текст ошибки без параметров `code` и `state`, чтобы уточнить поведение регистрации этого клиента.

## Проверенные источники

- [Frontier Developer Zone](https://auth.frontierstore.net/client/signup) и [документация Frontier](https://hosting.zaonce.net/docs/oauth2/instructions.html) — официальный протокол, scopes и PKCE; Developer Zone может требовать входа в аккаунт.
- [EDCD: заметки об OAuth Frontier](https://github.com/EDCD/FDevIDs/blob/master/Frontier%20API/FrontierDevelopments-oAuth2-notes.md) — Client ID, scopes, PKCE и передача `redirect_uri`.
- [Исходник авторизации EDMC](https://github.com/EDCD/EDMarketConnector/blob/main/companion.py) и [локальный callback EDMC](https://github.com/EDCD/EDMarketConnector/blob/main/protocol.py) — рабочая реализация нативного клиента и локального HTTP-возврата. Код EDMC в авторизацию ED Copilot не копировался.

Проверено 7 ноября 2026 года по текущей реализации EDMC и официальной документации Frontier. Форма Endpoints сверена с предоставленным владельцем приложения снимком экрана.
