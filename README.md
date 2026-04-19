# counter-service

Бэкенд-сервис для отслеживания просмотров и лайков постов. Демонстрирует production-grade паттерны: write-back кэширование, атомарные операции Redis, graceful shutdown и идемпотентность.

## Быстрый старт

### Предварительные требования

- Go 1.22+
- Docker & Docker Compose
- [goose](https://github.com/pressly/goose) для миграций

### Запуск

```bash
# 1. Клонировать репозиторий
git clone https://github.com/drenk83/counter-service.git
cd counter-service

# 2. Создать .env из примера
cp .env.example .env

# 3. Поднять Redis и Postgres
docker compose up -d

# 4. Применить миграции
goose -dir migrations postgres "postgres://pg:pg@localhost:5432/counter?sslmode=disable" up

# 5. Запустить сервис
go run ./cmd/server
```

Сервис будет доступен на `http://localhost:8080`.

## Архитектура

```
┌──────────┐   HTTP    ┌─────────┐   MarkViewed/IncrView   ┌───────┐
│  Client  │──────────▶│ Handler │──────────────────────────▶│ Redis │
└──────────┘           └────┬────┘                          └───┬───┘
                            │ GetStats                          │
                            │                              dirty set
                            ▼                                   │
                      ┌──────────┐    FlushStatsBatch    ┌──────▼──────┐
                      │ Postgres │◀──────────────────────│   Flusher   │
                      └──────────┘     (по тику)         └─────────────┘
```

**Ключевые принципы:**

- **Write-back cache** — все изменения счётчиков сначала попадают в Redis, а в Postgres сбрасываются фоновым воркером (`Flusher`) по расписанию.
- **Дедупликация просмотров** — Redis Set `viewed:{id}` с TTL 24 ч. гарантирует, что один пользователь засчитывается как один просмотр.
- **Идемпотентность лайков** — Set `liked:{id}` не даёт проставить лайк дважды от одного пользователя.
- **Dirty set** — список `dirty` в Redis хранит ID постов, у которых есть несброшенные дельты; Flusher вычитывает его атомарно через `SPOP`.
- **Атомарный flush** — `GETDEL` (Redis ≥ 6.2) забирает и обнуляет счётчик за одну операцию, исключая гонку read→delete.
- **Восстановление дельт** — если запись в Postgres упала, дельты возвращаются в Redis через `INCRBY` и пост снова добавляется в dirty set.
- **Graceful shutdown** — при получении SIGINT/SIGTERM HTTP-сервер дожидается завершения текущих запросов, Flusher завершает текущий цикл.

## Стек

| Компонент | Технология |
|---|---|
| HTTP-роутер | [chi](https://github.com/go-chi/chi) |
| Redis-клиент | [go-redis/v9](https://github.com/redis/go-redis) |
| Postgres-клиент | [pgx/v5](https://github.com/jackc/pgx) |
| Конфиг | `.env` + `godotenv` |
| Миграции | [goose](https://github.com/pressly/goose) |
| Инфраструктура | Docker Compose |

## Структура проекта

```
.
├── cmd/server/main.go          # Точка входа: сборка зависимостей, маршруты, запуск
├── internal/
│   ├── config/config.go        # Загрузка конфига из env
│   ├── handler/handler.go      # HTTP-хэндлеры
│   ├── repository/
│   │   ├── redis.go            # RedisRepo: счётчики, дедупликация, flush-примитивы
│   │   └── postgres.go         # PostgresRepo: источник истины, batch upsert
│   ├── service/counter.go      # Бизнес-логика: AddView, AddLike, GetStats
│   └── worker/flusher.go       # Фоновый воркер: сброс дельт из Redis в Postgres
├── migrations/                 # SQL-миграции для goose
└── docker-compose.yml
```

## API

Во всех мутирующих запросах обязателен заголовок `X-User-ID` — именно по нему производится дедупликация.

### POST `/posts/{id}/view`
Засчитывает просмотр поста. Повторный вызов от того же пользователя в течение 24 ч. игнорируется.

```bash
curl -X POST http://localhost:8080/posts/1/view \
     -H "X-User-ID: user-42"
# 204 No Content
```

### POST `/posts/{id}/like`
Ставит лайк. Идемпотентен: повторный лайк от того же пользователя не засчитывается.

```bash
curl -X POST http://localhost:8080/posts/1/like \
     -H "X-User-ID: user-42"
# 204 No Content
```

### DELETE `/posts/{id}/like`
Убирает лайк. Идемпотентен: если лайка не было — ничего не происходит.

```bash
curl -X DELETE http://localhost:8080/posts/1/like \
     -H "X-User-ID: user-42"
# 204 No Content
```

### GET `/posts/{id}/stats`
Возвращает актуальные счётчики: сумма данных из Redis (несброшенные дельты) и Postgres.

```bash
curl http://localhost:8080/posts/1/stats
```

```json
{
  "post_id": 1,
  "views": 1042,
  "likes": 87
}
```

### GET `/posts/batch?ids=1,2,3`
Батч-запрос статистики для нескольких постов. Использует `MGET` в Redis и `ANY($1)` в Postgres.

```bash
curl "http://localhost:8080/posts/batch?ids=1,2,3"
```

```json
[
  { "post_id": 1, "views": 1042, "likes": 87 },
  { "post_id": 2, "views": 530,  "likes": 41 },
  { "post_id": 3, "views": 0,    "likes": 0  }
]
```

## Конфигурация

Все параметры задаются через переменные окружения (или `.env`):

| Переменная | По умолчанию | Описание |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Адрес и порт HTTP-сервера |
| `REDIS_URL` | `redis://localhost:6379` | Подключение к Redis |
| `PG_DSN` | `postgres://pg:pg@localhost:5432/counter?sslmode=disable` | Подключение к Postgres |
| `FLUSH_TICK` | `30s` | Интервал сброса дельт в Postgres |

## Схема базы данных

```sql
CREATE TABLE posts (
    id         BIGSERIAL PRIMARY KEY,
    title      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE post_stats (
    post_id    BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    views      BIGINT NOT NULL DEFAULT 0,
    likes      BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`post_stats` обновляется через `INSERT ... ON CONFLICT DO UPDATE` — безопасный upsert без предварительного чтения строки.

## Ключевые паттерны реализации

### Write-back и dirty set

При каждом `AddView` / `AddLike` пост добавляется в Redis Set `dirty`. Flusher забирает ID через `SPOP`, получает дельты через пайплайн `GETDEL` и записывает их в Postgres батчем. Если Postgres недоступен, дельты возвращаются в Redis через `INCRBY`.

```
AddView(postID, userID)
  └─ SAdd viewed:{id} userID    ← дедупликация
  └─ INCR views:{id}            ← счётчик
  └─ SAdd dirty postID          ← помечаем как грязный
```

### Атомарный flush

```
Flusher.Flush()
  └─ SPOP dirty 10000           ← берём грязные ID
  └─ Pipeline GETDEL views:{id}
             GETDEL likes:{id}  ← забираем и обнуляем дельты атомарно
  └─ FlushStatsBatch(deltas)    ← batch upsert в Postgres
  └─ при ошибке: INCRBY + SAdd dirty  ← откат
```

### Агрегация при чтении

`GetStats` складывает данные из двух источников: несброшенные дельты из Redis и закоммиченные данные из Postgres. Пользователь всегда видит актуальные цифры независимо от того, был ли уже flush.
