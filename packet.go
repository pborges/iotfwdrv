package iotfwdrv

func ArgByName(p Packet, key string) interface{} {
	for _, arg := range p.Args() {
		if arg.Key == key {
			return arg.Value
		}
	}
	return nil
}

type Arg struct {
	Key   string
	Value interface{}
}

type Packet interface {
	Cmd() string
	Args() []Arg
}

type SetPacket struct {
	Key        string
	Value      interface{}
	Disconnect bool
}

func (p SetPacket) Cmd() string {
	return "set"
}

func (p SetPacket) Args() []Arg {
	return []Arg{
		{"name", p.Key},
		{"value", p.Value},
		{"disconnect", p.Disconnect},
	}
}

type PingPacket struct {
}

func (p PingPacket) Cmd() string {
	return "ping"
}

func (p PingPacket) Args() []Arg {
	return []Arg{}
}

type InfoPacket struct {
}

func (p InfoPacket) Cmd() string {
	return "info"
}

func (p InfoPacket) Args() []Arg {
	return []Arg{}
}

type ListPacket struct {
}

func (p ListPacket) Cmd() string {
	return "list"
}

func (p ListPacket) Args() []Arg {
	return []Arg{}
}

type SubscribePacket struct {
	Filter string
}

func (p SubscribePacket) Cmd() string {
	return "sub"
}

func (p SubscribePacket) Args() []Arg {
	return []Arg{
		{Key: "filter", Value: p.Filter},
	}
}

type rawPacket struct {
	cmd  string
	args map[string]interface{}
}

func (p *rawPacket) Set(key string, value interface{}) *rawPacket {
	if p.args == nil {
		p.args = make(map[string]interface{})
	}
	p.args[key] = value
	return p
}

func (p *rawPacket) Get(key string) interface{} {
	return p.args[key]
}

func (p *rawPacket) SetCommand(cmd string) *rawPacket {
	p.cmd = cmd
	return p
}

func (p rawPacket) Cmd() string {
	return p.cmd
}

func (p rawPacket) Args() []Arg {
	args := make([]Arg, 0, len(p.args))
	for k, v := range p.args {
		args = append(args, Arg{Key: k, Value: v})
	}
	return args
}

type BooleanAttributeUpdate struct {
	Name  string
	Value bool
}

func (p BooleanAttributeUpdate) Cmd() string {
	return "@attr"
}

func (p BooleanAttributeUpdate) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}

type IntegerAttributeUpdate struct {
	Name  string
	Value int64
}

func (p IntegerAttributeUpdate) Cmd() string {
	return "@attr"
}

func (p IntegerAttributeUpdate) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}

type DoubleAttributeUpdate struct {
	Name  string
	Value float64
}

func (p DoubleAttributeUpdate) Cmd() string {
	return "@attr"
}

func (p DoubleAttributeUpdate) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}

type UnsignedAttributeUpdate struct {
	Name  string
	Value uint64
}

func (p UnsignedAttributeUpdate) Cmd() string {
	return "@attr"
}

func (p UnsignedAttributeUpdate) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}

type StringAttributeUpdate struct {
	Name  string
	Value string
}

func (p StringAttributeUpdate) Cmd() string {
	return "@attr"
}

func (p StringAttributeUpdate) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}
