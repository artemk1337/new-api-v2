# New API V2

Улучшенная и доработанная версия исходного проекта
[new-api](https://github.com/QuantumNous/new-api): с исправлениями, улучшениями
стабильности и удобства развёртывания.

## Установка

Требуется Docker с Docker Compose.

```bash
git clone https://github.com/artemk1337/new-api-v2.git
cd new-api-v2
./install.sh
```

Скрипт выберет последнюю стабильную версию, сохранит её в `.env` и запустит
приложение с сервисом обновления. После запуска откройте
`http://localhost:3000`.

Для установки конкретной версии передайте тег:

```bash
./install.sh v1.1.9
```

## Обновление

Выполняйте обновление на Docker-хосте из каталога репозитория:

```bash
./install.sh update v1.1.105
```

Скрипт проверяет образ по тегу, создаёт и валидирует PostgreSQL-бэкап в
`backups/`, обновляет только `new-api` и при неуспешной проверке здоровья
восстанавливает предыдущую конфигурацию образа. Обновление из админки отключено:
оно должно выполняться только этим скриптом на Docker-хосте. Updater не
обновляется вместе с приложением. При необходимости его можно обновить отдельной
ручной командой:

```bash
./install.sh update-updater v1.1.105
```

Если на старой установке отсутствует контейнер telemetry agent, создайте его
с Docker-хоста из каталога репозитория:

```bash
./install.sh telemetry-agent
```

Команда проверяет Compose-конфигурацию и образ, затем делает `pull` и запускает
только `system-telemetry-agent` с `--no-deps`. Она не пересоздаёт `new-api`,
updater, PostgreSQL или Redis.

## Ручной запуск

Перед первым запуском измените пароли в `docker-compose.yml`, затем выполните:

```bash
docker compose up -d
```

Данные PostgreSQL и Redis хранятся в Docker-томах, а файлы приложения и логи —
в каталогах `data/` и `logs/` рядом с compose-файлом.

## Обновление production-установки

Используйте только готовые образы с явным тегом релиза. Перед обновлением
создайте и проверьте резервную копию базы данных; пересоздавайте только сервис
приложения и не удаляйте Docker-тома. Не запускайте на production-сервере
`docker build`, `docker compose build` или `docker compose up --build`.

### Billing details demo (local SQLite)

After starting the API once (so migrations create the database), seed an
isolated synthetic model, pricing options, and usage-log record:

```bash
./scripts/seed-billing-demo.sh              # uses SQLITE_PATH or one-api.db
# or: ./scripts/seed-billing-demo.sh /tmp/new-api-demo.db
```

Open the admin usage logs at `/usage-logs/common` and open request
`billing-demo-request-0001`. The script only replaces rows with its own demo
model/request identifiers and is safe to run repeatedly.
