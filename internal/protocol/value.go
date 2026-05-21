package protocol

type Type string

const (
	Array   Type = "*"
	Bulk    Type = "$"
	String  Type = "+"
	Error   Type = "-"
	Integer Type = ":"
	Null    Type = ""
)

type Value struct {
	Type  Type
	Bulk  string
	Str   string
	Err   string
	Int   int64
	Array []Value
}
