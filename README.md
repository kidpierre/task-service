# Task Service

Сервис управления задачами: регистрация и вход по JWT, создание и редактирование задач, список с фильтрами и пагинацией. Фоновая задача раз в минуту переводит задачи с истёкшим дедлайном в статус `overdue`.

Go 1.26, PostgreSQL 17. Из внешних библиотек только `pgx`, `golang-migrate`, `golang-jwt` и `bcrypt`, роутинг на стандартном `net/http`. В тестах testify и моки от mockery.

## Запуск

Нужен Docker с compose.

```bash
make up
```

Команда создаст `.env` со случайными паролем БД и `JWT_SECRET` (если файла ещё нет), соберёт образ и поднимет сервис вместе с Postgres. Миграции применяются при старте сервиса.

Без make:

```bash
cp .env.example .env
# вписать в .env POSTGRES_PASSWORD и JWT_SECRET (не короче 32 символов),
# например из openssl rand -hex 32
docker compose up --build -d
```

Сервис слушает `http://localhost:8080`, проверить можно через `curl localhost:8080/healthz`.

Остановить: `docker compose down`, вместе с данными: `docker compose down -v`.

Тесты:

```bash
go test -race ./...
```

Моки репозиториев лежат в `service/mocks_test.go`, после изменения интерфейсов их нужно перегенерировать через `make mocks`.

## Переменные окружения

Все переменные есть в `.env.example`. Обязательные только `DATABASE_URL` (в compose собирается из `POSTGRES_*`) и `JWT_SECRET`.

| Переменная | По умолчанию | |
|---|---|---|
| `HTTP_PORT` | `8080` | порт сервиса |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | таймауты HTTP-сервера |
| `HTTP_READ_TIMEOUT` | `10s` | |
| `HTTP_WRITE_TIMEOUT` | `10s` | |
| `HTTP_IDLE_TIMEOUT` | `60s` | |
| `DATABASE_URL` | | строка подключения к Postgres |
| `DB_MAX_OPEN_CONNS` | `25` | размер пула соединений |
| `DB_MAX_IDLE_CONNS` | `25` | |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | |
| `DB_PING_TIMEOUT` | `5s` | сколько ждать БД при старте |
| `JWT_SECRET` | | ключ подписи токенов, от 32 байт |
| `JWT_TTL` | `1h` | время жизни токена |
| `OVERDUE_CHECK_INTERVAL` | `1m` | как часто проверять просроченные задачи |
| `SHUTDOWN_TIMEOUT` | `10s` | сколько ждать активные запросы при остановке |
| `STOP_GRACE_PERIOD` | `20s` | сколько docker ждёт остановки контейнера |
| `LOG_LEVEL` | `info` | |

## API

Запросы и ответы в JSON, ошибки в виде `{"error": "..."}`. Даты в RFC 3339, например `2026-12-01T12:00:00Z`.

Для `/api/v1/tasks` нужен заголовок `Authorization: Bearer <token>`, токен выдаёт `/api/v1/auth/login`.

| | | |
|---|---|---|
| `POST /api/v1/auth/register` | регистрация | 201 |
| `POST /api/v1/auth/login` | вход | 200 |
| `POST /api/v1/tasks` | создать задачу | 201 |
| `GET /api/v1/tasks` | список задач | 200 |
| `GET /api/v1/tasks/{id}` | одна задача | 200 |
| `PATCH /api/v1/tasks/{id}` | изменить задачу | 200 |
| `DELETE /api/v1/tasks/{id}` | удалить задачу | 204 |
| `GET /healthz` | healthcheck | 200 |

Фильтры списка: `status`, `assignee_id`, `deadline_from`, `deadline_to`, пагинация через `limit` (по умолчанию 20, не больше 100) и `offset`. В ответе кроме `items` приходит `total`.

Статусы задач: `created`, `in_progress`, `done`, `overdue`.

Пример:

```bash
curl -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}'

TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' | jq -r .access_token)

curl -X POST localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Подготовить отчёт","description":"Квартальный","deadline":"2026-12-01T12:00:00Z"}'

curl "localhost:8080/api/v1/tasks?status=created&limit=10" -H "Authorization: Bearer $TOKEN"

curl -X PATCH localhost:8080/api/v1/tasks/1 \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress","assignee_id":2}'

curl -X DELETE localhost:8080/api/v1/tasks/1 -H "Authorization: Bearer $TOKEN"
```

Задача в ответе:

```json
{
  "id": 1,
  "title": "Подготовить отчёт",
  "description": "Квартальный",
  "status": "created",
  "deadline": "2026-12-01T12:00:00Z",
  "creator_id": 1,
  "assignee_id": 1,
  "created_at": "2026-10-13T10:00:00Z",
  "updated_at": "2026-10-13T10:00:00Z"
}
```

## Что не было описано в задании

- Задачи видны всем авторизованным пользователям, менять и удалять может только создатель. Ответственный тоже не может.
- Дедлайн при создании должен быть в будущем.
- Статус `overdue` ставит только фоновая задача, вручную его выставить нельзя. Если перенести дедлайн просроченной задачи, статус сам не вернётся, его нужно поменять в том же запросе.
- `PATCH` меняет только переданные поля.
- Пароль от 8 до 72 байт (ограничение bcrypt).

## Структура

```
cmd/            main
config/         чтение конфига из env
app/            сборка зависимостей, запуск и остановка
app/db/pg/      подключение к Postgres, миграции
entity/         модели и ошибки
service/        бизнес-логика
repository/pg/  запросы к БД
handler/http/   HTTP: роуты, middleware, DTO
worker/         фоновая задача
```

Слои общаются через интерфейсы, которые объявлены там, где используются. Всё собирается в `app/run.go`.

## Про безопасность

Пароли хранятся в bcrypt, SQL только с параметрами, права на изменение задачи проверяются по пользователю из токена. Токен подписывается HS256, другие алгоритмы при проверке не принимаются. На логине ответ для несуществующего email и неверного пароля одинаковый, в том числе по времени. Тело запроса ограничено 1 MB, лишние поля в JSON отклоняются.
