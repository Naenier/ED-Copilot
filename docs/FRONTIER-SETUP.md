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
2. Если карточка предлагает настройку `Redirect URI` / `Callback URL`, укажите точно:

   ```text
   http://127.0.0.1:7122/api/frontier/callback
   ```

   Это локальный адрес для возврата в запущенный ED Copilot. `Application Homepage URL` остаётся ссылкой на GitHub. В показанной начальной форме отдельного поля callback нет; не нужно заменять homepage локальным адресом. Некоторые варианты Developer Zone не предлагают отдельной регистрации callback: приложение само передаёт этот адрес в OAuth-запросе.
3. В ED Copilot откройте раздел авторизации Frontier, вставьте Client ID, сохраните и выберите вход через Frontier.
4. Оставьте ED Copilot запущенным, войдите на настоящем сайте Frontier и разрешите доступ. Браузер вернётся на локальный адрес; приложение проверит ответ и сохранит зашифрованные токены.
5. Обновите сведения CAPI в приложении. Появление Client ID ещё не подтверждает успешную авторизацию: она проверяется после этого входа.

Текущая реализация использует публичный клиент OAuth 2.0 с **PKCE S256**, scopes `auth capi`. **Shared Key / Client Secret вводить не требуется.** Не отправляйте Shared Key, токены, полный адрес callback с параметрами `code` или файлы профиля в GitHub. Сам Client ID не является паролем.

Если приложение запущено на другом локальном порту, используйте callback, который показывает само приложение, и тот же адрес в настройке клиента, если Frontier её предоставляет. Не заменяйте `127.0.0.1` на другой хост вручную.

## Проверенные источники

- [Frontier Developer Zone](https://auth.frontierstore.net/client/signup) и [документация Frontier](https://user.frontierstore.net/developer/docs) — страницы могут требовать входа в аккаунт.
- [EDCD: заметки об OAuth Frontier](https://github.com/EDCD/FDevIDs/blob/master/Frontier%20API/FrontierDevelopments-oAuth2-notes.md) — Client ID, scopes, PKCE и передача `redirect_uri`.
- [Исходник авторизации EDMC](https://github.com/EDCD/EDMarketConnector/blob/main/companion.py) и [локальный callback EDMC](https://github.com/EDCD/EDMarketConnector/blob/main/protocol.py) — рабочая реализация нативного клиента и локального HTTP-возврата. Код EDMC в авторизацию ED Copilot не копировался.

Проверено 7 ноября 2026 года. Детали формы после отправки недоступны без входа и могут отличаться; отдельный обязательный «второй экран callback» здесь не предполагается.
