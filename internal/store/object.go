package store

type ObjectType byte

const (
	TypeString ObjectType = iota
	TypeHash
	TypeList
)

func (t ObjectType) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeHash:
		return "hash"
	case TypeList:
		return "list"
	default:
		return "unknown"
	}
}

type Object struct {
	Type      ObjectType
	StringVal string
	HashVal   map[string]string
	ListVal   []string
	ExpiresAt int64
}

func NewString(s string) Object {
	return Object{Type: TypeString, StringVal: s}
}

func NewHash() Object {
	return Object{Type: TypeHash, HashVal: make(map[string]string)}
}

func NewList() Object {
	return Object{Type: TypeList, ListVal: nil}
}

func (o Object) approxSize() int {
	const base = 48
	switch o.Type {
	case TypeString:
		return base + len(o.StringVal)
	case TypeHash:
		sum := base + 48
		for k, v := range o.HashVal {
			sum += len(k) + len(v) + 16
		}
		return sum
	case TypeList:
		sum := base + 24
		for _, s := range o.ListVal {
			sum += len(s) + 16
		}
		return sum
	}
	return base
}
