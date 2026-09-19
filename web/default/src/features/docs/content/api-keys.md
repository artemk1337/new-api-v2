# Инструкция по подключению API-ключей

Здесь описаны выпуск API-ключа, подключение Claude Code и настройка клиентов с поддержкой OpenAI API. Команды установки сверены с официальной документацией [Setup](https://code.claude.com/docs/en/setup), [Authentication](https://code.claude.com/docs/en/authentication) и [Environment variables](https://code.claude.com/docs/en/env-vars).

> Токен вида `sk-xxxxxxxx` работает как пароль. Не публикуйте его в чатах, репозиториях, скриншотах или файлах с общим доступом. В примерах ниже стоит заглушка.

<h2 id="quick-start" class="scroll-mt-24">Быстрый старт</h2>

1. Зарегистрируйтесь на [панели](https://vibecode-api.online). Если включён вход через GitHub, пароль не понадобится.
2. Пополните баланс в разделе «Кошелёк» доступным способом.
3. В разделе «Ключи API» создайте токен и сохраните его в менеджере паролей.
4. Выберите адрес по типу клиента:

   | Клиент | Base URL |
   | --- | --- |
   | Claude Code и Claude CLI | `https://vibecode-api.online` без `/v1` |
   | Cursor, Cline, Roo Code, Continue, Open WebUI, Cherry Studio и другие OpenAI-compatible клиенты | `https://vibecode-api.online/v1` |

5. **Для Claude Code обязательно сначала настройте CLI:** задайте `ANTHROPIC_AUTH_TOKEN` и `ANTHROPIC_BASE_URL`, запустите `claude` и отправьте в CLI минимум один запрос. Дождитесь успешного ответа.
6. Только после первого успешного запроса в CLI открывайте приложение или интеграцию Claude Code.
7. В OpenAI-compatible клиенте укажите API Key и проверьте доступ к `/v1/models`.

Если нужен Claude Code, переходите к разделу [«Установка Claude Code CLI»](#claude-code-install). Для Cursor, Cline и других клиентов откройте [«OpenAI-compatible приложения»](#openai-compatible).

Claude Code использует Anthropic-совместимые переменные и адрес без `/v1`. OpenAI-compatible клиенты обращаются к `/v1/chat/completions`, поэтому им нужен адрес с `/v1`.

<h2 id="initial-setup" class="scroll-mt-24">1. Первичная настройка</h2>

1. Откройте [панель](https://vibecode-api.online/), зарегистрируйтесь или войдите.
2. Откройте раздел «Кошелёк» и выберите способ пополнения. Набор способов зависит от настроек сервиса и может включать СБП, банковскую карту, криптовалюту, ручной перевод или бонусный код.
3. Перейдите во вкладку «Ключи API» и создайте ключ.
4. Если доступен выбор группы, укажите нужную группу или `auto`. В режиме `auto` шлюз выбирает группу по правилам маршрутизации и при необходимости повторяет запрос в другой группе.
5. Скопируйте токен в менеджер паролей. Не добавляйте пробелы или переносы строк и не передавайте один ключ другим людям.
6. При необходимости задайте срок действия, квоту, список моделей и IP-адреса. Для разных устройств выпускайте отдельные ключи. Ограничение частоты запросов задаётся на уровне группы и отдельно для ключа не настраивается.

<h2 id="claude-code-install" class="scroll-mt-24">2. Установка Claude Code CLI</h2>

### macOS, Linux и WSL

#### 2.1. Нативный установщик

Он не требует Node.js и обновляется автоматически. Выполните команду в системном терминале:

```shell
curl -fsSL https://claude.ai/install.sh | bash
```

Скрипт загружается с домена `claude.ai` и сразу запускается. В корпоративной среде сначала сохраните его и проверьте по правилам безопасности. На macOS есть другой вариант:

```shell
brew install --cask claude-code
```

Homebrew обновляет такую установку командой `brew upgrade claude-code`.

После установки откройте новый терминал и проверьте CLI:

```shell
claude --version
claude doctor
```

Если команда `claude` не найдена, добавьте `~/.local/bin` в `PATH` по подсказке установщика и снова откройте терминал.

Альтернатива через npm нужна только при недоступном нативном установщике. Для актуального пакета требуется Node.js 22 или новее:

```shell
npm install -g @anthropic-ai/claude-code@latest
```

Не используйте `sudo npm install -g`, чтобы не получить проблемы с правами.

#### 2.2. Переменные окружения

Для zsh на macOS задайте переменные один раз:

```shell
echo 'export ANTHROPIC_AUTH_TOKEN="sk-xxxxxxxx"' >> ~/.zshrc
echo 'export ANTHROPIC_BASE_URL="https://vibecode-api.online"' >> ~/.zshrc
echo 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"' >> ~/.zshrc
source ~/.zshrc
```

Для bash используйте `~/.bashrc`:

```shell
echo 'export ANTHROPIC_AUTH_TOKEN="sk-xxxxxxxx"' >> ~/.bashrc
echo 'export ANTHROPIC_BASE_URL="https://vibecode-api.online"' >> ~/.bashrc
echo 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"' >> ~/.bashrc
source ~/.bashrc
```

#### 2.3. Файл `~/.claude/settings.json`

Выберите этот вариант, если переменные окружения не подхватываются или настройки удобнее хранить в одном файле. В файле будет секрет, поэтому не добавляйте его в репозиторий и ограничьте доступ.

```shell
mkdir -p ~/.claude
cd ~/.claude
touch settings.json
chmod 600 settings.json
```

Откройте `settings.json` в редакторе. На macOS подойдёт команда `open settings.json`, в Linux используйте привычный редактор.

Пример содержимого. Замените только значение токена:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-xxxxxxxx",
    "ANTHROPIC_BASE_URL": "https://vibecode-api.online",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` отключает телеметрию, отчёты об ошибках и автоматические обновления. При такой настройке периодически запускайте `claude update`. `API_TIMEOUT_MS` по умолчанию равен 600000 мс, то есть 10 минут. Добавляйте его только для другого таймаута. `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` нужен только при подтверждённой несовместимости шлюза с beta-заголовками.

Команды `cd`, `touch` и редактирование файла выполняются в системном терминале. При первом запуске подтвердите доверие к папке проекта. Если появится `Auth Conflict`, оставьте только `ANTHROPIC_AUTH_TOKEN` и удалите `ANTHROPIC_API_KEY` из окружения или настроек.

### Windows

1. Откройте PowerShell.
2. Установите Claude Code:

   ```powershell
   winget install Anthropic.ClaudeCode
   ```
3. Проверьте установку командами `claude --version` и `claude doctor`.
4. После изменения постоянных переменных через `setx` закройте терминал и откройте новый.

Если WinGet недоступен, используйте официальный PowerShell-установщик:

```powershell
irm https://claude.ai/install.ps1 | iex
```

Постоянные переменные через Command Prompt:

```bat
setx ANTHROPIC_AUTH_TOKEN "sk-xxxxxxxx"
setx ANTHROPIC_BASE_URL "https://vibecode-api.online"
setx CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC "1"
```

Только для текущей сессии PowerShell:

```powershell
$env:ANTHROPIC_AUTH_TOKEN="sk-xxxxxxxx"
$env:ANTHROPIC_BASE_URL="https://vibecode-api.online"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"
claude
```

Чтобы сохранить переменные из PowerShell без `setx`, используйте `[Environment]::SetEnvironmentVariable('ANTHROPIC_AUTH_TOKEN', 'sk-xxxxxxxx', 'User')` и так же задайте `ANTHROPIC_BASE_URL`. После этого откройте новый терминал.

<h2 id="claude-code" class="scroll-mt-24">3. Claude Code</h2>

Настройте CLI по предыдущему разделу, затем отправьте через него один запрос и дождитесь ответа.

1. Откройте папку проекта в системном терминале.
2. Запустите CLI:

   ```shell
   claude
   ```

3. Подтвердите доверие к папке и отправьте запрос, например: `Ответь одним словом: работает`.
4. Убедитесь, что модель успешно ответила. Ошибка авторизации, повторная попытка или зависший запрос не считаются успешной проверкой.
5. Закройте CLI и откройте приложение или интеграцию Claude Code.

Переменные окружения и `~/.claude/settings.json` применяются к CLI и интеграциям, которые запускают Claude Code CLI. Claude Desktop и облачные сессии на сайте могут использовать другие настройки.

При первом запуске проверьте путь к папке и доверяйте только каталогам с понятным содержимым.

> **Опасный режим.** `--dangerously-skip-permissions` отключает интерактивные подтверждения команд. Используйте его только в доверенной папке, не храните там секреты и проверяйте команды перед запуском.

Для доверенной папки:

```shell
claude --dangerously-skip-permissions
```

Не сохраняйте этот флаг в глобальном alias. Иначе его легко запустить в другой папке.

Модель можно сменить командой `/model`. В актуальных версиях Claude Code список моделей шлюза можно загрузить из `/v1/models` с помощью `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`. Сначала обновите CLI и убедитесь, что ваша версия поддерживает эту переменную.

<h2 id="openai-compatible" class="scroll-mt-24">4. OpenAI-compatible приложения</h2>

Выберите провайдера «OpenAI Compatible», укажите Base URL с `/v1` и вставьте API Key. Без Base URL приложение может обратиться к своему серверу по умолчанию.

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

| Симптом | Причина | Что сделать |
| --- | --- | --- |
| Claude Code обращается к `api.anthropic.com` | Не подхватился `ANTHROPIC_BASE_URL` | Проверьте окружение или `settings.json`, затем перезапустите терминал |
| `request to https://xxxx failed` | Неверный URL или лишний путь | Для Claude Code нужен URL без `/v1`, для других клиентов адрес с `/v1` |
| `Invalid token` или `401 token expired` | Неверный или истёкший токен | Проверьте ключ и срок действия. При необходимости выпустите новый и замените его во всех конфигурациях |
| `403` при корректном ключе | Токену запрещена группа, модель или IP-адрес | Проверьте группу, список разрешённых моделей, IP-ограничения и квоту в разделе **Ключи API** |
| `Auth conflict: ANTHROPIC_AUTH_TOKEN and ANTHROPIC_API_KEY` | Заданы две переменные авторизации | Оставьте `ANTHROPIC_AUTH_TOKEN`, удалите `ANTHROPIC_API_KEY` и откройте новый терминал. При необходимости выполните `/logout` |
| `Max tokens must exceed thinking budget` | `max_tokens` меньше заданного thinking budget | Увеличьте `max_tokens` или уменьшите параметр thinking/reasoning в клиенте |
| `API Error: retrying` или `overloaded_error` | Сеть, прокси или перегрузка сервера | Проверьте `/v1/models`, прокси и начните новую сессию после повторной попытки |
| `tool_call_error`, `400 tool_result` | Повреждено состояние диалога или несовместимы beta-заголовки | Повторите запрос и начните новую сессию. Флаг `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` задавайте только при подтверждённой связи ошибки со шлюзом |
| `npm WARN EBADENGINE` при установке | Используется Node.js старше требуемой версии | Используйте нативный установщик или Node.js 22 и новее |
| `File has been unexpectedly modified` | Две сессии меняют один проект | Закройте лишнюю сессию. В Windows используйте абсолютные пути |
| `ECONNRESET Retrying` | Соединение сброшено сетью, прокси или сервером | Повторите `/v1/models`, проверьте сеть и настройки прокси. Сам JSON-файл обычно не вызывает сброс TCP-соединения |
| Нужной модели нет в списке | Модель недоступна группе, запрещена ограничениями ключа или выключена на сервере | Сверьте `/v1/models`, группу и ограничения ключа. Обновление CLI само по себе не добавляет модели на сервер |
| `/v1/models` работает, приложение нет | Ошибка в провайдере, model ID, Base URL или авторизации | Проверьте четыре поля. `/v1` нужен OpenAI-compatible клиенту и не нужен в `ANTHROPIC_BASE_URL` |

<h2 id="commands" class="scroll-mt-24">8. Полезные команды Claude Code</h2>

| Команда | Назначение |
| --- | --- |
| `claude` | Запустить интерактивную сессию |
| `claude --dangerously-skip-permissions` | Запуск без лишних подтверждений в доверенной папке |
| `claude update` | Обновить Claude Code |
| `claude doctor` | Проверить установку, PATH и обновления |
| `claude --continue` (`-c`) | Продолжить последнюю сессию в текущей папке |
| `claude --resume` (`-r`) | Выбрать сессию из списка или продолжить её по ID/имени |
| `/exit` | Выйти из сессии |
| `/model` | Посмотреть или переключить модель |
| `/compact` | Сжать контекст диалога |
| `/logout` | Сбросить авторизацию при конфликте ключей |

<h2 id="hotkeys" class="scroll-mt-24">9. Горячие клавиши</h2>

| Клавиши | Действие |
| --- | --- |
| `Ctrl+C` | Отменить ввод или генерацию |
| `Ctrl+D` | Выйти из сессии |
| `Ctrl+L` | Очистить экран терминала |
| `↑` / `↓` | Навигация по истории команд |
| `Esc Esc` | Вернуться к предыдущему сообщению и изменить его |
| `Tab` | Автодополнение команд и путей |

<h2 id="context" class="scroll-mt-24">10. Как не потерять контекст</h2>

Для длинной задачи попросите Claude Code вести `todo.md` или `plan.md`. Записывайте туда цель, шаги, статус и решения. Перед `/compact` сохраните в файле важные требования. После сбоя запустите новую сессию и попросите продолжить работу по этому файлу.

<h2 id="faq" class="scroll-mt-24">11. FAQ</h2>

| Вопрос | Ответ |
| --- | --- |
| Можно ли вставить бонусный код вместо API-ключа? | Нет. Если бонусные коды доступны, применяйте их только в разделе «Кошелёк». В приложение вставляется токен `sk-...`. |
| Почему Claude Code использует URL без `/v1`? | Он работает через Anthropic-совместимые переменные и корневой URL шлюза. |
| Почему Cursor и Cline требуют `/v1`? | Они вызывают OpenAI-compatible адреса, например `/v1/chat/completions`. |
| Что выбрать: окружение или `settings.json`? | Окружение удобно для одной сессии, а `~/.claude/settings.json` для постоянной настройки. В обоих случаях секрет хранится локально. |
| Что делать, если `/v1/models` работает, а приложение нет? | Сверьте провайдер, Base URL и model ID. Для OpenAI-compatible клиента проверьте Bearer-токен, для Anthropic-клиента `x-api-key`, `ANTHROPIC_AUTH_TOKEN` и `anthropic-version`. |
| Почему после установки через npm появляется `EBADENGINE`? | Актуальный npm-пакет требует Node.js 22 или новее. Используйте нативный установщик или обновите Node.js. |
| Можно ли запускать две сессии в одном проекте? | Не рекомендуется, потому что они могут одновременно менять файлы. |
| Как обратиться в поддержку? | Передайте тип клиента, Base URL, текст ошибки и результат проверки без реального токена. |
| Что делать при утечке токена? | Немедленно удалите или перевыпустите ключ в панели и замените его во всех конфигурациях. |
| Нужен ли прокси? | Обычно нет. Задавайте `HTTP_PROXY` или `HTTPS_PROXY` только для известного прокси и порта. |

<h2 id="support-checklist" class="scroll-mt-24">12. Чек-лист перед обращением за помощью</h2>

- [ ] Токен начинается с `sk-` и не содержит пробелов.
- [ ] Для Claude Code используется `https://vibecode-api.online` без `/v1`.
- [ ] Для OpenAI-compatible приложений используется `https://vibecode-api.online/v1`.
- [ ] После изменения постоянного окружения открыт новый терминал или перезапущен клиент.
- [ ] `/v1/models` возвращает список моделей или понятную ошибку.
- [ ] Проверены срок действия, квота, группа, разрешённые модели и IP-ограничения токена.
- [ ] Нет одновременных `ANTHROPIC_AUTH_TOKEN` и `ANTHROPIC_API_KEY`.
- [ ] В проекте не запущены две конфликтующие сессии.
- [ ] `claude --version` и `claude doctor` выполняются без критичных ошибок.
- [ ] На Windows проверены `PATH` и `where.exe claude`.

<h2 id="compatible-apps" class="scroll-mt-24">13. Совместимые приложения</h2>

Claude Code CLI подключается через Anthropic Messages API. Cursor, Cline, Roo Code, Continue.dev, Cherry Studio, Open WebUI, OpenHands, Aider и другие клиенты могут использовать OpenAI Compatible API, если в их версии можно указать свой Base URL и API Key.

<h2 id="url-reference" class="scroll-mt-24">14. Шпаргалка по URL</h2>

| Сценарий | URL | Примечание |
| --- | --- | --- |
| Claude Code: Base URL | `https://vibecode-api.online` | Используйте корень без `/v1`. CLI сам вызывает Anthropic-адреса |
| Anthropic Messages API | `https://vibecode-api.online/v1/messages` | Обычно клиент формирует этот путь сам |
| OpenAI-compatible: Base URL | `https://vibecode-api.online/v1` | Для клиентов с отдельным полем Base URL |
| OpenAI Chat Completions | `https://vibecode-api.online/v1/chat/completions` | Полный адрес нужен только если клиент просит endpoint, а не Base URL |
| OpenAI Responses API | `https://vibecode-api.online/v1/responses` | Для клиентов с поддержкой Responses API |
| Список моделей | `https://vibecode-api.online/v1/models` | Поддерживает Bearer и Anthropic-style авторизацию |
