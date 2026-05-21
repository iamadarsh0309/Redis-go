package command

import (
	"log"
	"strconv"
	"time"

	"go-redis/internal/protocol"
)

func cmdCommand(e *Engine, v *protocol.Value) *protocol.Value { return okReply() }

func cmdPing(e *Engine, v *protocol.Value) *protocol.Value {
	if len(v.Array) >= 2 {
		return bulkReply(v.Array[1].Bulk)
	}
	return &protocol.Value{Type: protocol.String, Str: "PONG"}
}

func cmdExists(e *Engine, v *protocol.Value) *protocol.Value {
	if len(v.Array) < 2 {
		return errReply("ERR wrong number of arguments for 'exists'")
	}
	var n int64
	for _, a := range v.Array[1:] {
		if e.store.Exists(a.Bulk) {
			n++
		}
	}
	return intReply(n)
}

func cmdDel(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 1 {
		return errReply("ERR wrong number of arguments for 'del'")
	}
	var deleted int64
	for _, a := range args {
		if e.store.Delete(a.Bulk) {
			deleted++
		}
	}
	if deleted > 0 {
		logCommand(e, v)
	}
	return intReply(deleted)
}

func cmdExpire(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 2 {
		return errReply("ERR wrong number of arguments for 'expire'")
	}
	secs, err := strconv.ParseInt(args[1].Bulk, 10, 64)
	if err != nil {
		return errReply("ERR value is not an integer or out of range")
	}
	absMs := time.Now().UnixMilli() + secs*1000
	if !e.store.SetExpiry(args[0].Bulk, absMs) {
		return intReply(0)
	}
	logPexpireAt(e, args[0].Bulk, absMs)
	return intReply(1)
}

func cmdPExpireAt(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 2 {
		return errReply("ERR wrong number of arguments for 'pexpireat'")
	}
	ms, err := strconv.ParseInt(args[1].Bulk, 10, 64)
	if err != nil {
		return errReply("ERR value is not an integer or out of range")
	}
	if !e.store.SetExpiry(args[0].Bulk, ms) {
		return intReply(0)
	}
	logCommand(e, v)
	return intReply(1)
}

func cmdTTL(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'ttl'")
	}
	ms, ok := ttlMs(e, args[0].Bulk)
	if !ok {
		return intReply(-2)
	}
	if ms < 0 {
		return intReply(-1)
	}
	return intReply(ms / 1000)
}

func cmdPTTL(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'pttl'")
	}
	ms, ok := ttlMs(e, args[0].Bulk)
	if !ok {
		return intReply(-2)
	}
	return intReply(ms)
}

func ttlMs(e *Engine, key string) (int64, bool) {
	obj, ok := e.store.Get(key)
	if !ok {
		return 0, false
	}
	if obj.ExpiresAt == 0 {
		return -1, true
	}
	return obj.ExpiresAt - time.Now().UnixMilli(), true
}

func cmdPersist(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'persist'")
	}
	if !e.store.Persist(args[0].Bulk) {
		return intReply(0)
	}
	logCommand(e, v)
	return intReply(1)
}

func logCommand(e *Engine, v *protocol.Value) {
	if e.aof == nil {
		return
	}
	if err := e.aof.Append(v); err != nil {
		log.Println("aof append failed:", err)
	}
}

func logPexpireAt(e *Engine, key string, absMs int64) {
	if e.aof == nil {
		return
	}
	cmd := &protocol.Value{
		Type: protocol.Array,
		Array: []protocol.Value{
			{Type: protocol.Bulk, Bulk: "PEXPIREAT"},
			{Type: protocol.Bulk, Bulk: key},
			{Type: protocol.Bulk, Bulk: strconv.FormatInt(absMs, 10)},
		},
	}
	if err := e.aof.Append(cmd); err != nil {
		log.Println("aof append failed:", err)
	}
}
