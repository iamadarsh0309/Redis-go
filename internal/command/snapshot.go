package command

import (
	"strconv"

	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

func Snapshot(s store.Store) []*protocol.Value {
	out := make([]*protocol.Value, 0, s.Len())
	s.Each(func(key string, obj store.Object) {
		switch obj.Type {
		case store.TypeString:
			out = append(out, makeArrayCmd("SET", key, obj.StringVal))
		case store.TypeHash:
			if cmd := makeHSetCmd(key, obj.HashVal); cmd != nil {
				out = append(out, cmd)
			}
		case store.TypeList:
			if cmd := makeRPushCmd(key, obj.ListVal); cmd != nil {
				out = append(out, cmd)
			}
		}
		if obj.ExpiresAt > 0 {
			out = append(out, makeArrayCmd("PEXPIREAT", key, strconv.FormatInt(obj.ExpiresAt, 10)))
		}
	})
	return out
}

func makeArrayCmd(parts ...string) *protocol.Value {
	arr := make([]protocol.Value, len(parts))
	for i, p := range parts {
		arr[i] = protocol.Value{Type: protocol.Bulk, Bulk: p}
	}
	return &protocol.Value{Type: protocol.Array, Array: arr}
}

func makeHSetCmd(key string, fields map[string]string) *protocol.Value {
	if len(fields) == 0 {
		return nil
	}
	arr := make([]protocol.Value, 0, 2+2*len(fields))
	arr = append(arr,
		protocol.Value{Type: protocol.Bulk, Bulk: "HSET"},
		protocol.Value{Type: protocol.Bulk, Bulk: key},
	)
	for f, v := range fields {
		arr = append(arr,
			protocol.Value{Type: protocol.Bulk, Bulk: f},
			protocol.Value{Type: protocol.Bulk, Bulk: v},
		)
	}
	return &protocol.Value{Type: protocol.Array, Array: arr}
}

func makeRPushCmd(key string, items []string) *protocol.Value {
	if len(items) == 0 {
		return nil
	}
	arr := make([]protocol.Value, 0, 2+len(items))
	arr = append(arr,
		protocol.Value{Type: protocol.Bulk, Bulk: "RPUSH"},
		protocol.Value{Type: protocol.Bulk, Bulk: key},
	)
	for _, s := range items {
		arr = append(arr, protocol.Value{Type: protocol.Bulk, Bulk: s})
	}
	return &protocol.Value{Type: protocol.Array, Array: arr}
}
