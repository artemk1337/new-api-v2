## Подробная инструкция по API-ключам и CLI

Здесь описаны выпуск API-ключа, подключение Claude Code и настройка клиентов с поддержкой OpenAI API. Команды установки сверены с официальной документацией [Setup](https://code.claude.com/docs/en/setup), [Authentication](https://code.claude.com/docs/en/authentication) и [Environment variables](https://code.claude.com/docs/en/env-vars).

> Токен вида `sk-xxxxxxxx` работает как пароль. Не публикуйте его в чатах, репозиториях, скриншотах или файлах с общим доступом. В примерах ниже стоит заглушка.

<h2 id="quick-start" class="scroll-mt-24">Быстрый старт</h2>

1. [Войдите в кабинет](/dashboard).
2. [Пополните баланс в «Кошельке»](/wallet).
3. [В «Ключах API» создайте и скопируйте ключ](/keys).
4. Вставьте настройки из таблицы в приложение и выберите модель.

<h2 id="claude-code-install" class="scroll-mt-24">2. Установка Claude Code CLI</h2>

Выберите свою ОС и выполните шаги в её разделе.

### macOS

1. Откройте приложение «Терминал» и установите Claude Code:

   ```shell
   curl -fsSL https://claude.ai/install.sh | bash
   ```

2. Откройте новое окно терминала и проверьте установку:

   ```shell
   claude --version
   claude doctor
   ```

Если пользуетесь Homebrew, вместо команды из первого шага можно выполнить `brew install --cask claude-code`. Обновление такой установки выполняется командой `brew upgrade claude-code`.

### Linux и WSL

В Windows с WSL выполняйте эти шаги в терминале Linux-дистрибутива, например Ubuntu.

1. Откройте терминал Linux и установите Claude Code:

   ```shell
   curl -fsSL https://claude.ai/install.sh | bash
   ```

2. Откройте новый терминал и проверьте установку:

   ```shell
   claude --version
   claude doctor
   ```

Если команда `claude` не найдена на macOS или Linux, добавьте `~/.local/bin` в `PATH` по подсказке установщика и снова откройте терминал.

Нативный установщик для macOS и Linux обновляется автоматически и не требует Node.js. Скрипт загружается с домена `claude.ai` и сразу запускается. В корпоративной среде сначала сохраните его и проверьте по правилам безопасности.

### Windows без WSL

1. Откройте PowerShell и установите Claude Code:

   ```powershell
   winget install Anthropic.ClaudeCode
   ```

   Если WinGet недоступен, используйте вместо него официальный PowerShell-установщик:

   ```powershell
   irm https://claude.ai/install.ps1 | iex
   ```

2. Откройте новое окно PowerShell и проверьте установку:

   ```powershell
   claude --version
   claude doctor
   ```

### Запасной способ через npm

Используйте его, если основной способ для вашей ОС недоступен.

1. Установите Node.js 22 или новее.
2. Установите Claude Code:

   ```shell
   npm install -g @anthropic-ai/claude-code@latest
   ```

   На macOS и Linux не используйте `sudo npm install -g`, чтобы не получить проблемы с правами.

3. Откройте новый терминал и проверьте установку командами `claude --version` и `claude doctor`.

<h2 id="claude-code" class="scroll-mt-24">3. Подключение Claude Code</h2>

Сначала [установите Claude Code](#claude-code-install).

1. Откройте или создайте файл настроек в домашней папке пользователя:

   - macOS, Linux и WSL: `~/.claude/settings.json`
   - Windows: `%USERPROFILE%\.claude\settings.json`

   Если папки `.claude` нет, создайте её.

2. Добавьте настройки. Замените `sk-xxxxxxxx` своим API-ключом. Если файл уже содержит настройки, добавьте поля в существующий блок `env`.

   ```json
   {
     "env": {
       "ANTHROPIC_AUTH_TOKEN": "sk-xxxxxxxx",
       "ANTHROPIC_BASE_URL": "https://vibecode-api.online"
     }
   }
   ```

   Адрес указывайте без `/v1`. Храните ключ в личном файле настроек вне репозитория.

3. Откройте терминал в папке проекта и запустите Claude Code:

   ```shell
   claude
   ```

4. Отправьте любой запрос и дождитесь ответа модели.

Модель можно сменить командой `/model`. При ошибке подключения откройте [диагностику](#diagnostics).

Для работы в редакторе используйте [инструкцию для VS Code](#claude-code-vscode).

<h2 id="openai-compatible" class="scroll-mt-24">4. OpenAI-compatible приложения</h2>

Выберите провайдера «OpenAI Compatible», укажите Base URL с `/v1` и вставьте API Key. Для клиентов с полями API Host и API Path используйте настройки в блоках [«Для компьютера»](#desktop-apps) и [«Для телефона»](#mobile-apps). Без Base URL приложение может обратиться к своему серверу по умолчанию.

Подключить можно Cursor, Cline, Roo Code, Continue.dev, Open WebUI, Cherry Studio, OpenHands и Aider. Названия полей зависят от версии клиента.

Base URL: `https://vibecode-api.online/v1`

Пример Continue.dev:

```yaml
models:
  - title: Claude через API-шлюз
    provider: openai
    model: MODEL_ID_FROM_V1_MODELS
    apiBase: https://vibecode-api.online/v1
    apiKey: sk-xxxxxxxx
```

Точный model ID возьмите из ответа `/v1/models` или из списка моделей в форме создания или редактирования ключа. Общая административная страница моделей может быть недоступна обычному пользователю.

### Codex CLI

Подключите Codex CLI как отдельного провайдера и укажите API-ключ в `~/.codex/config.toml`:

```toml
model_provider = "custom"

[model_providers.custom]
name = "VibeCode API"
base_url = "https://vibecode-api.online/v1"
experimental_bearer_token = "sk-xxxxxxxx"
wire_api = "responses"
```

`experimental_bearer_token` хранит ключ прямо в конфиге. Ограничьте доступ к файлу:

```shell
chmod 600 ~/.codex/config.toml
```

Замените `sk-xxxxxxxx` на реальный ключ и не добавляйте файл в репозиторий.

Дополнительные параметры custom providers описаны в [официальной документации Codex](https://developers.openai.com/codex/config-file/config-advanced#custom-model-providers).

### Cline, Cursor и другие клиенты

Проверьте четыре поля: тип провайдера `OpenAI Compatible`, Base URL с `/v1`, API Key с токеном `sk-...` и точный model ID из `/v1/models`. Не угадывайте имя модели и не добавляйте префиксы вручную. Поддержка собственного адреса зависит от версии и тарифа клиента.

<h2 id="diagnostics" class="scroll-mt-24">5. Проверка ключа и диагностика</h2>

Чтобы ключ не попал в историю shell, прочитайте его в скрытую переменную и проверьте список моделей:

```shell
read -s VIBECODE_API_KEY
curl --fail-with-body --silent --show-error \
  https://vibecode-api.online/v1/models \
  -H "Authorization: Bearer ${VIBECODE_API_KEY}"
unset VIBECODE_API_KEY
```

Этот запрос использует `Authorization: Bearer`. Для Anthropic-совместимой авторизации используйте тот же адрес с заголовками Claude API:

```shell
read -s VIBECODE_API_KEY
curl --fail-with-body --silent --show-error \
  https://vibecode-api.online/v1/models \
  -H "x-api-key: ${VIBECODE_API_KEY}" \
  -H 'anthropic-version: 2023-06-01'
unset VIBECODE_API_KEY
```

| Проверка | Команда или действие | Нормальный результат |
| --- | --- | --- |
| Claude Code | `claude --version` и `claude doctor` | Версия выводится, критичных ошибок нет |
| Claude Code в PATH | macOS/Linux: `command -v claude`; Windows: `where.exe claude` | Один ожидаемый путь. Для нативной установки обычно `~/.local/bin/claude` |
| Base URL | macOS/Linux: `printf '%s\n' "$ANTHROPIC_BASE_URL"`; PowerShell: `$env:ANTHROPIC_BASE_URL` | Для Claude Code: `https://vibecode-api.online` |
| Доступность моделей | Один из запросов выше с `${VIBECODE_API_KEY}` | OpenAI JSON для Bearer или Anthropic JSON для `x-api-key` |
| Токен и группа | Панель «Ключи API» | Токен активен, срок и квота не исчерпаны, группа и модели подходят запросу |

<h2 id="troubleshooting" class="scroll-mt-24">6. Алгоритм решения проблем</h2>

1. Определите место сбоя: панель и токен, сеть, окружение, Claude Code или конкретный клиент.
2. Проверьте Base URL: без `/v1` для Claude Code и с `/v1` для OpenAI-compatible клиентов.
3. Проверьте токен. Он должен начинаться с `sk-` и не содержать пробелов, кавычек или переносов строк. Срок, квота, группа, список моделей и IP-ограничения тоже должны разрешать запрос.
4. После изменения постоянных переменных откройте новый терминал или перезапустите приложение.
5. Выполните запрос к `/v1/models`. Если он не работает, сначала исправьте адрес, сеть или токен.
6. При ошибке в длинном диалоге попробуйте `/compact`, `/exit` или новую сессию. `Esc Esc` вернёт к предыдущему сообщению для редактирования.
7. Если сбой повторяется только в одном проекте, проверьте вторую сессию Claude Code, конфликтующие файлы и права доступа.

<h2 id="error-table" class="scroll-mt-24">7. Таблица ошибок</h2>

| Код | Текст | Причина и что сделать |
| --- | --- | --- |
| - | Claude Code обращается к `api.anthropic.com` | Не применился `ANTHROPIC_BASE_URL`. Проверьте окружение или `settings.json` и перезапустите терминал. |
| - | `request to https://xxxx failed` | Неверный URL или лишний путь. Сверьте адрес с инструкцией приложения. Для Claude Code укажите адрес без `/v1`. |
| `401` | `Invalid token`, `token expired` | Ключ неверный или истёк. Проверьте ключ и срок действия, при необходимости создайте новый. |
| `403` | Запрос отклонён при корректном ключе | Ключу запрещена группа, модель или IP-адрес. Проверьте ограничения и квоту в разделе «Ключи API». |
| - | `Auth conflict: ANTHROPIC_AUTH_TOKEN and ANTHROPIC_API_KEY` | Заданы две переменные авторизации. Оставьте `ANTHROPIC_AUTH_TOKEN`, удалите `ANTHROPIC_API_KEY` и откройте новый терминал. При необходимости выполните `/logout`. |
| - | `Max tokens must exceed thinking budget` | `max_tokens` меньше thinking budget. Увеличьте лимит ответа или уменьшите thinking/reasoning в клиенте. |
| - | `API Error: retrying`, `overloaded_error` | Сбой сети, прокси или перегрузка сервера. Проверьте `/v1/models` и прокси, повторите запрос в новой сессии. |
| `400` | `tool_result` | Повреждён диалог или несовместимы beta-заголовки. Начните новую сессию. Задавайте `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` только при подтверждённой связи ошибки со шлюзом. |
| `tool_call_error` | Ошибка вызова инструмента | Повторите запрос в новой сессии. Проверьте совместимость модели и инструментов клиента. |
| `EBADENGINE` | `npm WARN EBADENGINE` | Версия Node.js не подходит. Используйте нативный установщик или Node.js 22 и новее. |
| - | `File has been unexpectedly modified` | Две сессии меняют один проект. Закройте лишнюю сессию. В Windows используйте абсолютные пути. |
| `ECONNRESET` | `Retrying` | Соединение сброшено. Повторите `/v1/models`, проверьте сеть и прокси. |
| - | Нужной модели нет в списке | Модель недоступна группе, ограничена ключом или выключена на сервере. Проверьте `/v1/models`, группу и ограничения ключа. |
| - | `/v1/models` работает, приложение нет | Проверьте провайдера, model ID, адрес и ключ. Значения полей возьмите из инструкции приложения. |

<h2 id="faq" class="scroll-mt-24">11. FAQ</h2>

| Вопрос | Ответ |
| --- | --- |
| Нужен ли VPN для использования из России? | Нет, можно подключаться напрямую. |
| С чего начать? | Откройте [«Быстрый старт»](#quick-start). Там шаги и настройки подключения. |
| Где взять API-ключ? | Создайте и скопируйте его в разделе [«Ключи API»](/keys). |
| Можно пользоваться с телефона? | Да. Например, через [Chatbox AI](#mobile-chatbox) на iPhone и Android. Другие приложения есть в меню «Для телефона». |
| Какой адрес указать в приложении? | Скопируйте его из инструкции своего приложения в меню. Base URL, API Host и API Path заполняются по-разному. |
| Как пополнить баланс? | Откройте [«Кошелёк»](/wallet) и выберите способ пополнения. |
| Где посмотреть цены? | На странице [«Модели»](/pricing). |
| Можно использовать один ключ для разных моделей? | Да, если эти модели разрешены для вашего ключа. |
| Почему нужной модели нет в списке? | Проверьте, доступна ли она вашему ключу. Затем обновите список моделей в приложении. Подробнее в [проверке ключа](#diagnostics). |
| Что делать при ошибке? | Найдите код или текст сообщения в [таблице ошибок](#error-table). |
| Что отправить в поддержку? | Название приложения, текст ошибки и скриншот настроек со скрытым API-ключом. |

<h2 id="support-checklist" class="scroll-mt-24">12. Чек-лист поиска неисправности</h2>

1. Токен начинается с `sk-` и не содержит пробелов.
2. Для Claude Code используется `https://vibecode-api.online` без `/v1`.
3. Для OpenAI-compatible приложений используется `https://vibecode-api.online/v1`.
4. После изменения постоянного окружения открыт новый терминал или перезапущен клиент.
5. `/v1/models` возвращает список моделей или понятную ошибку.
6. Проверены срок действия, квота, группа, разрешённые модели и IP-ограничения токена.
7. Нет одновременных `ANTHROPIC_AUTH_TOKEN` и `ANTHROPIC_API_KEY`.
8. В проекте не запущены две конфликтующие сессии.
9. `claude --version` и `claude doctor` выполняются без критичных ошибок.
10. На Windows проверены `PATH` и `where.exe claude`.
