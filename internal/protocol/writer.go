package protocol

import (
	"bufio"
	"fmt"
	"io"
)

type Writer struct {
	bw *bufio.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriter(w)}
}

func (w *Writer) Serialize(v *Value) string {
	switch v.Type {
	case Array:
		out := fmt.Sprintf("*%d\r\n", len(v.Array))
		for i := range v.Array {
			out += w.Serialize(&v.Array[i])
		}
		return out
	case String:
		return fmt.Sprintf("+%s\r\n", v.Str)
	case Bulk:
		return fmt.Sprintf("$%d\r\n%s\r\n", len(v.Bulk), v.Bulk)
	case Error:
		return fmt.Sprintf("-%s\r\n", v.Err)
	case Integer:
		return fmt.Sprintf(":%d\r\n", v.Int)
	case Null:
		return "$-1\r\n"
	default:
		return ""
	}
}

func (w *Writer) Write(v *Value) error {
	_, err := w.bw.WriteString(w.Serialize(v))
	return err
}

func (w *Writer) Flush() error {
	return w.bw.Flush()
}
