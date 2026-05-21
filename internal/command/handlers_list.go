package command

import (
	"strconv"

	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

func cmdLPush(e *Engine, v *protocol.Value) *protocol.Value {
	return pushHandler(e, v, true, "lpush")
}

func cmdRPush(e *Engine, v *protocol.Value) *protocol.Value {
	return pushHandler(e, v, false, "rpush")
}

func pushHandler(e *Engine, v *protocol.Value, left bool, name string) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 2 {
		return errReply("ERR wrong number of arguments for '" + name + "'")
	}
	key := args[0].Bulk
	values := args[1:]

	var newLen int64
	var wrongType bool
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if present && obj.Type != store.TypeList {
			wrongType = true
			return false, false
		}
		if !present {
			*obj = store.NewList()
		}
		obj.ListVal = pushValues(obj.ListVal, values, left)
		newLen = int64(len(obj.ListVal))
		return true, false
	})
	if wrongType {
		return wrongTypeReply()
	}
	logCommand(e, v)
	return intReply(newLen)
}

func pushValues(list []string, vals []protocol.Value, left bool) []string {
	if left {
		head := make([]string, len(vals))
		for i, v := range vals {
			head[len(vals)-1-i] = v.Bulk
		}
		return append(head, list...)
	}
	for _, val := range vals {
		list = append(list, val.Bulk)
	}
	return list
}

func cmdLRange(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 3 {
		return errReply("ERR wrong number of arguments for 'lrange'")
	}
	start, err1 := strconv.ParseInt(args[1].Bulk, 10, 64)
	stop, err2 := strconv.ParseInt(args[2].Bulk, 10, 64)
	if err1 != nil || err2 != nil {
		return errReply("ERR value is not an integer or out of range")
	}

	var out []protocol.Value
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeList {
			wrongType = true
			return
		}
		n := int64(len(obj.ListVal))
		s, t := normalizeRange(start, stop, n)
		if s > t {
			return
		}
		out = make([]protocol.Value, 0, t-s+1)
		for _, item := range obj.ListVal[s : t+1] {
			out = append(out, protocol.Value{Type: protocol.Bulk, Bulk: item})
		}
	})
	if wrongType {
		return wrongTypeReply()
	}
	return arrayReply(out)
}

func normalizeRange(start, stop, n int64) (int64, int64) {
	if start < 0 {
		start = max64(start+n, 0)
	}
	if stop < 0 {
		stop = stop + n
	}
	if start >= n {
		return 1, 0
	}
	if stop >= n {
		stop = n - 1
	}
	return start, stop
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func cmdLPop(e *Engine, v *protocol.Value) *protocol.Value { return popHandler(e, v, true, "lpop") }

func cmdRPop(e *Engine, v *protocol.Value) *protocol.Value { return popHandler(e, v, false, "rpop") }

func popHandler(e *Engine, v *protocol.Value, left bool, name string) *protocol.Value {
	args := v.Array[1:]
	if len(args) < 1 || len(args) > 2 {
		return errReply("ERR wrong number of arguments for '" + name + "'")
	}
	withCount := len(args) == 2
	count := int64(1)
	if withCount {
		c, err := strconv.ParseInt(args[1].Bulk, 10, 64)
		if err != nil || c < 0 {
			return errReply("ERR value is out of range, must be positive")
		}
		count = c
	}

	var popped []string
	var wrongType bool
	e.store.Update(args[0].Bulk, func(obj *store.Object, present bool) (bool, bool) {
		if !present {
			return false, false
		}
		if obj.Type != store.TypeList {
			wrongType = true
			return false, false
		}
		actual := count
		if actual > int64(len(obj.ListVal)) {
			actual = int64(len(obj.ListVal))
		}
		if actual == 0 {
			return false, false
		}
		popped = make([]string, actual)
		if left {
			copy(popped, obj.ListVal[:actual])
			obj.ListVal = obj.ListVal[actual:]
		} else {
			tail := obj.ListVal[int64(len(obj.ListVal))-actual:]
			for i, val := range tail {
				popped[int64(len(tail))-1-int64(i)] = val
			}
			obj.ListVal = obj.ListVal[:int64(len(obj.ListVal))-actual]
		}
		if len(obj.ListVal) == 0 {
			return false, true
		}
		return true, false
	})
	if wrongType {
		return wrongTypeReply()
	}
	if len(popped) > 0 {
		logCommand(e, v)
	}

	if !withCount {
		if len(popped) == 0 {
			return nullReply()
		}
		return bulkReply(popped[0])
	}
	if len(popped) == 0 {
		return nullReply()
	}
	out := make([]protocol.Value, len(popped))
	for i, p := range popped {
		out[i] = protocol.Value{Type: protocol.Bulk, Bulk: p}
	}
	return arrayReply(out)
}

func cmdLLen(e *Engine, v *protocol.Value) *protocol.Value {
	args := v.Array[1:]
	if len(args) != 1 {
		return errReply("ERR wrong number of arguments for 'llen'")
	}
	var n int64
	var wrongType bool
	e.store.View(args[0].Bulk, func(obj store.Object, present bool) {
		if !present {
			return
		}
		if obj.Type != store.TypeList {
			wrongType = true
			return
		}
		n = int64(len(obj.ListVal))
	})
	if wrongType {
		return wrongTypeReply()
	}
	return intReply(n)
}

func applyPush(e *Engine, v *protocol.Value, left bool) {
	if len(v.Array) < 3 {
		return
	}
	key := v.Array[1].Bulk
	values := v.Array[2:]
	e.store.Update(key, func(obj *store.Object, present bool) (bool, bool) {
		if present && obj.Type != store.TypeList {
			return false, false
		}
		if !present {
			*obj = store.NewList()
		}
		obj.ListVal = pushValues(obj.ListVal, values, left)
		return true, false
	})
}

func applyPop(e *Engine, v *protocol.Value, left bool) {
	if len(v.Array) < 2 {
		return
	}
	count := int64(1)
	if len(v.Array) >= 3 {
		if c, err := strconv.ParseInt(v.Array[2].Bulk, 10, 64); err == nil && c >= 0 {
			count = c
		}
	}
	e.store.Update(v.Array[1].Bulk, func(obj *store.Object, present bool) (bool, bool) {
		if !present || obj.Type != store.TypeList {
			return false, false
		}
		actual := count
		if actual > int64(len(obj.ListVal)) {
			actual = int64(len(obj.ListVal))
		}
		if actual == 0 {
			return false, false
		}
		if left {
			obj.ListVal = obj.ListVal[actual:]
		} else {
			obj.ListVal = obj.ListVal[:int64(len(obj.ListVal))-actual]
		}
		if len(obj.ListVal) == 0 {
			return false, true
		}
		return true, false
	})
}
