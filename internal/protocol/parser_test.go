package protocol

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseSimpleArray(t *testing.T) {
	in := "*2\r\n$3\r\nGET\r\n$1\r\nx\r\n"
	p := NewParser(strings.NewReader(in))
	v, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v.Type != Array || len(v.Array) != 2 {
		t.Fatalf("expected array of 2, got %+v", v)
	}
	if v.Array[0].Bulk != "GET" || v.Array[1].Bulk != "x" {
		t.Fatalf("bad contents: %+v", v.Array)
	}
}

func TestParseMultiDigitArrayLength(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("*12\r\n")
	for i := 0; i < 12; i++ {
		sb.WriteString("$1\r\na\r\n")
	}
	p := NewParser(strings.NewReader(sb.String()))
	v, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Array) != 12 {
		t.Fatalf("expected 12 elements, got %d", len(v.Array))
	}
}

func TestParseLongBulkString(t *testing.T) {
	body := strings.Repeat("x", 1000)
	in := "$1000\r\n" + body + "\r\n"
	p := NewParser(strings.NewReader(in))
	v, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v.Bulk != body {
		t.Fatalf("bulk mismatch: len=%d expected=%d", len(v.Bulk), len(body))
	}
}

func TestParsePipelining(t *testing.T) {
	in := "*1\r\n$4\r\nPING\r\n*2\r\n$3\r\nGET\r\n$1\r\nk\r\n"
	p := NewParser(strings.NewReader(in))
	v1, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v1.Array[0].Bulk != "PING" {
		t.Fatalf("first cmd: %+v", v1)
	}
	v2, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v2.Array[0].Bulk != "GET" || v2.Array[1].Bulk != "k" {
		t.Fatalf("second cmd: %+v", v2)
	}
}

func TestParseNullBulk(t *testing.T) {
	p := NewParser(strings.NewReader("$-1\r\n"))
	v, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v.Type != Null {
		t.Fatalf("expected Null, got %v", v.Type)
	}
}

func TestParseSimpleString(t *testing.T) {
	p := NewParser(strings.NewReader("+OK\r\n"))
	v, err := p.ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if v.Type != String || v.Str != "OK" {
		t.Fatalf("got %+v", v)
	}
}

func TestParseInteger(t *testing.T) {
	cases := map[string]int64{
		":0\r\n":              0,
		":1\r\n":              1,
		":-42\r\n":            -42,
		":9999999999\r\n":     9999999999,
		":-9999999999\r\n":    -9999999999,
	}
	for in, want := range cases {
		v, err := NewParser(strings.NewReader(in)).ReadValue()
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if v.Type != Integer || v.Int != want {
			t.Fatalf("%q: got %+v, want Int=%d", in, v, want)
		}
	}
}

func TestSerializeInteger(t *testing.T) {
	var sb strings.Builder
	w := NewWriter(&sb)
	w.Write(&Value{Type: Integer, Int: 42})
	w.Flush()
	if got := sb.String(); got != ":42\r\n" {
		t.Fatalf("got %q, want %q", got, ":42\r\n")
	}
}

func TestParseEOFAfterCompleteValue(t *testing.T) {
	p := NewParser(strings.NewReader("+OK\r\n"))
	if _, err := p.ReadValue(); err != nil {
		t.Fatal(err)
	}
	_, err := p.ReadValue()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestParseMalformed(t *testing.T) {
	cases := map[string]string{
		"non-numeric array length": "*not_a_number\r\n",
		"truncated bulk body":      "$3\r\nab",
		"unknown prefix":           "weird\r\n",
		"missing CRLF":             "+OK\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewParser(strings.NewReader(in))
			if _, err := p.ReadValue(); err == nil {
				t.Errorf("expected error for input %q", in)
			}
		})
	}
}

func TestParseRoundTripWithWriter(t *testing.T) {
	src := &Value{
		Type: Array,
		Array: []Value{
			{Type: Bulk, Bulk: "SET"},
			{Type: Bulk, Bulk: "key-with-12345-digits"},
			{Type: Bulk, Bulk: strings.Repeat("v", 500)},
		},
	}
	var sb strings.Builder
	w := NewWriter(&sb)
	if err := w.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}

	got, err := NewParser(strings.NewReader(sb.String())).ReadValue()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Array) != 3 ||
		got.Array[0].Bulk != "SET" ||
		got.Array[1].Bulk != "key-with-12345-digits" ||
		got.Array[2].Bulk != src.Array[2].Bulk {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
