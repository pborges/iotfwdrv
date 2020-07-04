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

type RawPacket struct {
	cmd  string
	args map[string]interface{}
}

func (p *RawPacket) Set(key string, value interface{}) *RawPacket {
	if p.args == nil {
		p.args = make(map[string]interface{})
	}
	p.args[key] = value
	return p
}

func (p *RawPacket) SetCommand(cmd string) *RawPacket {
	p.cmd = cmd
	return p
}

func (p RawPacket) Cmd() string {
	return p.cmd
}

func (p RawPacket) Args() []Arg {
	args := make([]Arg, 0, len(p.args))
	for k, v := range p.args {
		args = append(args, Arg{Key: k, Value: v})
	}
	return args
}
