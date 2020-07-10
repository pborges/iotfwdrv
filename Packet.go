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

type AttributeUpdatePacket struct {
	Name  string
	Type  string
	Value interface{}
}

func (p AttributeUpdatePacket) Cmd() string {
	return "@attr"
}

func (p AttributeUpdatePacket) Args() []Arg {
	return []Arg{
		{Key: "name", Value: p.Name},
		{Key: "type", Value: p.Name},
		{Key: "value", Value: p.Value},
	}
}
