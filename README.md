# Redis-Go

A Redis-compatible in-memory data store, written from scratch in Go. Speaks the standard RESP protocol on port 6379, so any Redis client — `redis-cli`, `go-redis`, `redis-py`, `node-redis`, etc. — works against it out of the box.

```
┌─ Your app ────────┐         ┌─ redis-go ─────────────────────┐
│ go-redis          │ ◄──TCP──► RESP parser → command engine    │
│ redis-py          │  6379    → in-memory store + AOF + RDB    │
│ redis-cli         │          └────────────────────────────────┘
└───────────────────┘
```

For internals, design decisions, and the codebase tour, see [docs/](docs/) — the project's "book."

---

## Install & run

```bash
git clone <repo-url> redis-go
cd redis-go
go run ./cmd/server       # starts on :6379
```

Or build a binary:

```bash
go build -o redis-go ./cmd/server
./redis-go                # uses ./redis.conf if present, else defaults
```

Requirements: Go 1.22+.

Stop with `Ctrl-C` (SIGINT) or `kill <pid>` (SIGTERM) — both trigger a graceful shutdown that drains in-flight connections and flushes the AOF before exit.

---

## Connect from your code

### `redis-cli`

```bash
redis-cli -h 127.0.0.1 -p 6379
127.0.0.1:6379> SET name adarsh
OK
127.0.0.1:6379> GET name
"adarsh"
127.0.0.1:6379> EXPIRE name 60
(integer) 1
127.0.0.1:6379> TTL name
(integer) 58
```

### Go (`go-redis/v9`)

```bash
go get github.com/redis/go-redis/v9
```

```go
package main

import (
    "context"
    "fmt"
    "github.com/redis/go-redis/v9"
)

func main() {
    rdb := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    ctx := context.Background()

    rdb.Set(ctx, "name", "adarsh", 0)
    name, _ := rdb.Get(ctx, "name").Result()
    fmt.Println(name)

    rdb.HSet(ctx, "user:1", "email", "a@b.com", "role", "admin")
    fields, _ := rdb.HGetAll(ctx, "user:1").Result()
    fmt.Println(fields)
}
```

### Python (`redis-py`)

```bash
pip install redis
```

```python
import redis

r = redis.Redis(host="localhost", port=6379, decode_responses=True)

r.set("name", "adarsh")
print(r.get("name"))

r.rpush("queue", "a", "b", "c")
print(r.lrange("queue", 0, -1))    # ['a', 'b', 'c']

r.expire("name", 60)
print(r.ttl("name"))               # 60
```

### Node.js (`node-redis`)

```bash
npm install redis
```

```javascript
import { createClient } from 'redis';

const client = createClient({ url: 'redis://localhost:6379' });
await client.connect();

await client.set('name', 'adarsh');
console.log(await client.get('name'));

await client.hSet('user:1', { email: 'a@b.com', role: 'admin' });
console.log(await client.hGetAll('user:1'));
```

### Raw RESP (for debugging)

```bash
(printf '*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$5\r\nhello\r\n'; sleep 0.1; \
 printf '*2\r\n$3\r\nGET\r\n$1\r\nk\r\n') | nc -w 2 localhost 6379
```

---

## Configuration (`redis.conf`)

Place a `redis.conf` next to the binary or in the working directory. Missing file → defaults.

```conf
dir ./data                    # directory for AOF and RDB files

# Append-Only File (write-ahead log)
appendonly yes                # yes | no
appendfilename backup.aof
appendfsync everysec          # always | everysec | no
                              #   always   = fsync per write (zero data loss, slow)
                              #   everysec = fsync once a second (≤1 s data loss)
                              #   no       = OS decides (fastest, biggest window)

# RDB snapshots (point-in-time dump)
save 900 1                    # snapshot if ≥1 key changed in 900s
save 300 10                   # ... or ≥10 keys in 300s
save 60  10000                # ... or ≥10000 keys in 60s
dbfilename backup.rdb

# Memory bound + eviction (optional)
maxkeys 1000000               # 0 = unbounded
maxmemory-policy noeviction   # noeviction | allkeys-random | volatile-ttl
```

All values fall back to safe defaults; you only need to set what you care about.

---

## Supported commands

### Strings
| Command | Example |
|---|---|
| `GET key` | `GET name` |
| `SET key value` | `SET name adarsh` — resets any prior TTL |

### Hashes
| Command | Example |
|---|---|
| `HSET key field value [field value ...]` | `HSET user:1 email a@b.com role admin` |
| `HGET key field` | `HGET user:1 email` |
| `HMGET key field [field ...]` | `HMGET user:1 email role` |
| `HGETALL key` | `HGETALL user:1` |
| `HDEL key field [field ...]` | `HDEL user:1 role` (drops key when last field removed) |
| `HLEN key` / `HEXISTS key field` | |

### Lists
| Command | Example |
|---|---|
| `LPUSH key value [value ...]` | `LPUSH queue a b c` → head = `[c, b, a]` |
| `RPUSH key value [value ...]` | `RPUSH queue a b c` → tail = `[..., a, b, c]` |
| `LRANGE key start stop` | `LRANGE queue 0 -1` (negative indices supported) |
| `LPOP key [count]` / `RPOP key [count]` | drops key when list becomes empty |
| `LLEN key` | |

### TTL / generic
| Command | Notes |
|---|---|
| `EXISTS key [key ...]` | count of present keys |
| `DEL key [key ...]` | count of deleted keys |
| `EXPIRE key seconds` | logged as `PEXPIREAT key absMs` for durable wall-clock TTL |
| `PEXPIREAT key ms` | absolute deadline form |
| `TTL key` / `PTTL key` | `-2` missing, `-1` no TTL, else remaining |
| `PERSIST key` | clears TTL |
| `PING [message]` | |

### Admin / persistence
| Command | Notes |
|---|---|
| `DBSIZE` | total keys |
| `FLUSHDB` | remove every key (logged to AOF) |
| `SAVE` | synchronous RDB snapshot (blocks the calling connection) |
| `BGSAVE` | background RDB snapshot |
| `BGREWRITEAOF` | compact the AOF — preserves concurrent writes via a diff buffer |
| `LASTSAVE` | unix timestamp of last successful save |
| `INFO [section]` | sections: `server` / `memory` / `stats` / `keyspace` / `all` |
| `COMMAND` | returns `OK` (stub for client handshakes) |

### Not yet supported (compared to real Redis)
- `INCR`, `DECR`, `APPEND`, `STRLEN`, `GETSET`
- Sets (`SADD`, `SMEMBERS`, …)
- Sorted Sets (`ZADD`, `ZRANGE`, …)
- Pub/Sub (`SUBSCRIBE`, `PUBLISH`)
- Transactions (`MULTI`, `EXEC`, `WATCH`)
- Lua scripting
- Streams (`XADD`, `XREAD`)
- Cluster mode (single-node only)
- ACL / `AUTH`
- Client tracking

---

## Persistence at a glance

| File | Written by | Loaded on startup if |
|---|---|---|
| `<dir>/<appendfilename>` (AOF) | every mutation, when `appendonly yes` | `appendonly yes` |
| `<dir>/<dbfilename>` (RDB) | `SAVE` / `BGSAVE` / periodic `save N M` | `appendonly no` and file exists |

The AOF is replayed on startup to reconstruct in-memory state. Periodic `save N M` rules trigger a background `BGSAVE`. `BGREWRITEAOF` compacts the AOF without blocking writes (concurrent appends are captured in a diff buffer and folded into the new file before an atomic rename).

---

## Development

```bash
go test ./... -race          # all unit tests with race detector
go vet ./...
gofmt -w .
go run ./cmd/server          # live server
```

Layout:

```
cmd/server/                  process entrypoint
internal/protocol/           RESP parser + serializer + Value type
internal/store/              in-memory storage engine (Object, TTL, eviction)
internal/persistence/        AOF + RDB
internal/command/            command dispatch + per-type handlers + INFO/admin
internal/config/             redis.conf parser
docs/                        the project's "book" (internals, ADRs, Q&A, code tour)
```

To understand the internals deeply, start with [docs/SYSTEM_DESIGN.md](docs/SYSTEM_DESIGN.md) and [docs/CODE_TOUR.md](docs/CODE_TOUR.md).

---

## License

MIT.
