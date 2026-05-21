package command

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"go-redis/internal/persistence"
	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

type Engine struct {
	store     store.Store
	aof       persistence.Logger
	startTime time.Time

	rdbPath     string
	lastSaveMu  sync.Mutex
	lastSaveAt  time.Time
	bgSaving    bool
	bgRewriting bool
}

func NewEngine(s store.Store, aof persistence.Logger) *Engine {
	return &Engine{store: s, aof: aof, startTime: time.Now(), lastSaveAt: time.Now()}
}

func (e *Engine) Store() store.Store { return e.store }

func (e *Engine) SetRDBPath(path string) { e.rdbPath = path }

func (e *Engine) LastSave() time.Time {
	e.lastSaveMu.Lock()
	defer e.lastSaveMu.Unlock()
	return e.lastSaveAt
}

type Handler func(e *Engine, v *protocol.Value) *protocol.Value

var handlers = map[string]Handler{
	"COMMAND":      cmdCommand,
	"PING":         cmdPing,
	"EXISTS":       cmdExists,
	"DEL":          cmdDel,
	"EXPIRE":       cmdExpire,
	"PEXPIREAT":    cmdPExpireAt,
	"TTL":          cmdTTL,
	"PTTL":         cmdPTTL,
	"PERSIST":      cmdPersist,
	"INFO":         cmdInfo,
	"DBSIZE":       cmdDBSize,
	"FLUSHDB":      cmdFlushDB,
	"SAVE":         cmdSave,
	"BGSAVE":       cmdBGSave,
	"BGREWRITEAOF": cmdBGRewriteAOF,
	"LASTSAVE":     cmdLastSave,
	"GET":          cmdGet,
	"SET":          cmdSet,
	"HSET":         cmdHSet,
	"HGET":         cmdHGet,
	"HMGET":        cmdHMGet,
	"HGETALL":      cmdHGetAll,
	"HDEL":         cmdHDel,
	"HLEN":         cmdHLen,
	"HEXISTS":      cmdHExists,
	"LPUSH":        cmdLPush,
	"RPUSH":        cmdRPush,
	"LRANGE":       cmdLRange,
	"LPOP":         cmdLPop,
	"RPOP":         cmdRPop,
	"LLEN":         cmdLLen,
}

func (e *Engine) Execute(v *protocol.Value) *protocol.Value {
	if len(v.Array) == 0 {
		return errReply("ERR empty command")
	}
	cmd := strings.ToUpper(v.Array[0].Bulk)
	h, ok := handlers[cmd]
	if !ok {
		return errReply("ERR unknown command '" + cmd + "'")
	}
	return h(e, v)
}

func (e *Engine) Apply(v *protocol.Value) {
	if len(v.Array) == 0 {
		return
	}
	cmd := strings.ToUpper(v.Array[0].Bulk)
	switch cmd {
	case "SET":
		if len(v.Array) >= 3 {
			e.store.Set(v.Array[1].Bulk, store.NewString(v.Array[2].Bulk))
		}
	case "DEL":
		for i := 1; i < len(v.Array); i++ {
			e.store.Delete(v.Array[i].Bulk)
		}
	case "PEXPIREAT":
		if len(v.Array) >= 3 {
			ms, err := strconv.ParseInt(v.Array[2].Bulk, 10, 64)
			if err == nil {
				e.store.SetExpiry(v.Array[1].Bulk, ms)
			}
		}
	case "PERSIST":
		if len(v.Array) >= 2 {
			e.store.Persist(v.Array[1].Bulk)
		}
	case "HSET":
		applyHSet(e, v)
	case "HDEL":
		applyHDel(e, v)
	case "LPUSH":
		applyPush(e, v, true)
	case "RPUSH":
		applyPush(e, v, false)
	case "LPOP":
		applyPop(e, v, true)
	case "RPOP":
		applyPop(e, v, false)
	case "FLUSHDB":
		e.store.FlushAll()
	}
}

func errReply(msg string) *protocol.Value {
	return &protocol.Value{Type: protocol.Error, Err: msg}
}

func okReply() *protocol.Value {
	return &protocol.Value{Type: protocol.String, Str: "OK"}
}

func intReply(n int64) *protocol.Value {
	return &protocol.Value{Type: protocol.Integer, Int: n}
}

func bulkReply(s string) *protocol.Value {
	return &protocol.Value{Type: protocol.Bulk, Bulk: s}
}

func nullReply() *protocol.Value {
	return &protocol.Value{Type: protocol.Null}
}

func arrayReply(vs []protocol.Value) *protocol.Value {
	return &protocol.Value{Type: protocol.Array, Array: vs}
}

func wrongTypeReply() *protocol.Value {
	return errReply("WRONGTYPE Operation against a key holding the wrong kind of value")
}
