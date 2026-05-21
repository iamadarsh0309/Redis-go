package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

type Parser struct {
	r *bufio.Reader
}

func NewParser(r io.Reader) *Parser {
	if br, ok := r.(*bufio.Reader); ok {
		return &Parser{r: br}
	}
	return &Parser{r: bufio.NewReader(r)}
}

func (p *Parser) ReadValue() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	if len(line) == 0 {
		return nil, fmt.Errorf("empty RESP line")
	}
	switch line[0] {
	case '*':
		return p.parseArray(line[1:])
	case '$':
		return p.parseBulk(line[1:])
	case '+':
		return &Value{Type: String, Str: string(line[1:])}, nil
	case '-':
		return &Value{Type: Error, Err: string(line[1:])}, nil
	case ':':
		n, err := strconv.ParseInt(string(line[1:]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", line[1:], err)
		}
		return &Value{Type: Integer, Int: n}, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix: %q", line[0])
	}
}

func (p *Parser) readLine() ([]byte, error) {
	line, err := p.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("malformed RESP line: missing CRLF")
	}
	return line[:len(line)-2], nil
}

func (p *Parser) parseArray(lenBytes []byte) (*Value, error) {
	n, err := strconv.Atoi(string(lenBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid array length %q: %w", lenBytes, err)
	}
	if n < 0 {
		return &Value{Type: Null}, nil
	}
	v := &Value{Type: Array, Array: make([]Value, n)}
	for i := 0; i < n; i++ {
		sub, err := p.ReadValue()
		if err != nil {
			return nil, err
		}
		v.Array[i] = *sub
	}
	return v, nil
}

func (p *Parser) parseBulk(lenBytes []byte) (*Value, error) {
	n, err := strconv.Atoi(string(lenBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid bulk length %q: %w", lenBytes, err)
	}
	if n < 0 {
		return &Value{Type: Null}, nil
	}
	buf := make([]byte, n+2)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return nil, fmt.Errorf("reading bulk body: %w", err)
	}
	if buf[n] != '\r' || buf[n+1] != '\n' {
		return nil, fmt.Errorf("bulk string missing trailing CRLF")
	}
	return &Value{Type: Bulk, Bulk: string(buf[:n])}, nil
}
