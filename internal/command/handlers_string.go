package command

import (
	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

func cmdGet(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'get'")
	}
	obj, ok := e.store.Get(args[0].Bulk)
	if !ok {
		return nullReply()
	}
	if obj.Type != store.TypeString {
		return wrongTypeReply()
	}
	return bulkReply(obj.StringVal)
}

func cmdSet(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 2 {
		return errReply("ERR wrong number of arguments for 'set'")
	}
	e.store.Set(args[0].Bulk, store.NewString(args[1].Bulk))
	logCommand(e, v)
	return okReply()
}
