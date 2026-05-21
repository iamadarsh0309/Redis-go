package command

import (
	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

func cmdHSet(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 3 || len(args)%2 != 1 {
		return errReply("ERR wrong number of arguments for 'hset'")
	}
	key := args[0].Bulk
	pairs := args[1:]

	var added int64
	var wrongType bool
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if present && obj.Type != store.TypeHash {
			wrongType = true
			return false, false
		}
		if !present {
			*obj = store.NewHash()
		}
		for i := 0; i < len(pairs); i += 2 {
			f, val := pairs[i].Bulk, pairs[i+1].Bulk
			if _, exists := obj.HashVal[f]; !exists {
				added++
			}
			obj.HashVal[f] = val
		}
		return true, false
	})
	if wrongType {
		return wrongTypeReply()
	}
	logCommand(e, v)
	return intReply(added)
}

func cmdHGet(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 2 {
		return errReply("ERR wrong number of arguments for 'hget'")
	}
	var (
		val       string
		ok        bool
		wrongType bool
	)
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeHash {
			wrongType = true
			return
		}
		val, ok = obj.HashVal[args[1].Bulk]
	})
	if wrongType {
		return wrongTypeReply()
	}
	if !ok {
		return nullReply()
	}
	return bulkReply(val)
}

func cmdHMGet(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 2 {
		return errReply("ERR wrong number of arguments for 'hmget'")
	}
	out := make([]protocol.Value, len(args)-1)
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if present && obj.Type != store.TypeHash {
			wrongType = true
			return
		}
		for i, field := range args[1:] {
			if !present {
				out[i] = protocol.Value{Type: protocol.Null}
				continue
			}
			if val, ok := obj.HashVal[field.Bulk]; ok {
				out[i] = protocol.Value{Type: protocol.Bulk, Bulk: val}
			} else {
				out[i] = protocol.Value{Type: protocol.Null}
			}
		}
	})
	if wrongType {
		return wrongTypeReply()
	}
	return arrayReply(out)
}

func cmdHGetAll(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'hgetall'")
	}
	var out []protocol.Value
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeHash {
			wrongType = true
			return
		}
		out = make([]protocol.Value, 0, 2*len(obj.HashVal))
		for f, val := range obj.HashVal {
			out = append(out,
				protocol.Value{Type: protocol.Bulk, Bulk: f},
				protocol.Value{Type: protocol.Bulk, Bulk: val},
			)
		}
	})
	if wrongType {
		return wrongTypeReply()
	}
	return arrayReply(out)
}

func cmdHDel(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 2 {
		return errReply("ERR wrong number of arguments for 'hdel'")
	}
	key := args[0].Bulk
	fields := args[1:]
	var removed int64
	var wrongType bool
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if !present {
			return false, false
		}
		if obj.Type != store.TypeHash {
			wrongType = true
			return false, false
		}
		for _, f := range fields {
			if _, exists := obj.HashVal[f.Bulk]; exists {
				delete(obj.HashVal, f.Bulk)
				removed++
			}
		}
		if len(obj.HashVal) == 0 {
			return false, true
		}
		return removed > 0, false
	})
	if wrongType {
		return wrongTypeReply()
	}
	if removed > 0 {
		logCommand(e, v)
	}
	return intReply(removed)
}

func cmdHLen(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'hlen'")
	}
	var n int64
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeHash {
			wrongType = true
			return
		}
		n = int64(len(obj.HashVal))
	})
	if wrongType {
		return wrongTypeReply()
	}
	return intReply(n)
}

func cmdHExists(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 2 {
		return errReply("ERR wrong number of arguments for 'hexists'")
	}
	var found bool
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeHash {
			wrongType = true
			return
		}
		_, found = obj.HashVal[args[1].Bulk]
	})
	if wrongType {
		return wrongTypeReply()
	}
	if found {
		return intReply(1)
	}
	return intReply(0)
}

func applyHSet(e *Engine, v *protocol.Value) {
	if len(v.Array) < 4 || (len(v.Array)-2)%2 != 0 {
		return
	}
	key := v.Array[1].Bulk
	pairs := v.Array[2:]
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if present && obj.Type != store.TypeHash {
			return false, false
		}
		if !present {
			*obj = store.NewHash()
		}
		for i := 0; i < len(pairs); i += 2 {
			obj.HashVal[pairs[i].Bulk] = pairs[i+1].Bulk
		}
		return true, false
	})
}

func applyHDel(e *Engine, v *protocol.Value) {
	if len(v.Array) < 3 {
		return
	}
	key := v.Array[1].Bulk
	fields := v.Array[2:]
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if !present || obj.Type != store.TypeHash {
			return false, false
		}
		for _, f := range fields {
			delete(obj.HashVal, f.Bulk)
		}
		if len(obj.HashVal) == 0 {
			return false, true
		}
		return true, false
	})
}
