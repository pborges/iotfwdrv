package iotfwdrv

import (
	"errors"
	"fmt"
	"strings"
)

func Encode(p Packet) string {
	s := make([]string, 0, len(p.Args())+1)
	s = append(s, sanitize(p.Cmd()))
	if p.Args != nil {
		for _, v := range p.Args() {
			s = append(s, encodeKey(v.Key, v.Value))
		}
	}
	return fmt.Sprint(strings.Join(s, " "))
}

func encodeKey(key string, value interface{}) string {
	return fmt.Sprintf("%s:%s", sanitize(key), sanitize(value))
}

func sanitize(s interface{}) string {
	str := fmt.Sprintf("%v", s)
	if strings.Contains(str, " ") {
		return fmt.Sprintf("\"%s\"", str)
	}
	return str
}

type tokenizerState int

var tokDecodeCommand tokenizerState = 0
var tokDecodeKey tokenizerState = 1
var tokDecodeValue tokenizerState = 2

func Decode(str string) (Packet, error) {
	var state tokenizerState
	var p RawPacket
	var key string
	var value string
	var inQuote bool
	for _, c := range str {
		switch state {
		case tokDecodeCommand:
			if c != ' ' {
				p.SetCommand(p.Cmd() + string(c))
			} else {
				state++
			}
		case tokDecodeKey:
			switch c {
			case '"':
				if inQuote {
					inQuote = false
				} else {
					inQuote = true
				}
			case ' ':
				if !inQuote {
					return p, errors.New("unexpected space in key")
				}
				key += string(c)
			case ':':
				if inQuote {
					return p, errors.New("unclosed quote")
				}
				inQuote = false
				state = tokDecodeValue
			default:
				key += string(c)
			}
		case tokDecodeValue:
			switch c {
			case '"':
				if inQuote {
					inQuote = false
				} else {
					inQuote = true
				}
			case ' ':
				if !inQuote {
					p.Set(key, value)
					key, value = "", ""
					inQuote = false
					state = tokDecodeKey
				} else {
					value += string(c)
				}
			default:
				value += string(c)
			}
		}
	}
	if key != "" {
		p.args[key] = value
	}
	return p, nil
}
