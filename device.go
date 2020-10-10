package iotfwdrv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"
)

var ErrNotConnected = errors.New("not connected")
var ErrAlreadyStarted = errors.New("device already started")

type Message struct {
	Message string
	Value   string
}

type Metadata struct {
	Key   string
	Value string
}

type Info struct {
	ID          string
	Name        string
	Model       string
	FirmwareVer Version
	HardwareVer Version
	Metadata    []Metadata
}

type Device struct {
	dialer             func() (io.ReadWriteCloser, error)
	execCh             chan func()
	inbound            chan string
	conn               io.ReadWriteCloser
	connected          bool
	setup              sync.Once
	subscriptions      []*Subscription
	subscriptionsSetup bool
	metadata           map[string]string
	Log                *log.Logger
}

func (dev *Device) Connected() bool {
	return dev.connected
}

func (dev *Device) SetMetadata(key, value string) {
	dev.exec(func() {
		dev.metadata[key] = value
	})
}

func (dev *Device) Start(dialer func() (io.ReadWriteCloser, error)) error {
	var setup bool
	dev.setup.Do(func() {
		setup = true
		if dev.Log == nil {
			dev.Log = log.New(ioutil.Discard, "[iotfwdrv] ", log.LstdFlags)
		}
		dev.metadata = make(map[string]string)
		dev.execCh = make(chan func())
		dev.inbound = make(chan string)
		dev.dialer = dialer

		go dev.execHandler()
		go func() {
			for {
				dev.reader()
			}
		}()
	})
	if !setup && dev.dialer != nil {
		return ErrAlreadyStarted
	}
	return nil
}

func (dev *Device) synchronousWrite(cmd packet) (res []packet, err error) {
	dev.exec(func() {
		res, err = dev.write(cmd)
	})
	return
}

func (dev *Device) Set(name string, value interface{}) (err error) {
	_, err = dev.synchronousWrite(packet{
		Cmd: "set",
		Args: map[string]string{
			"name":  name,
			"value": fmt.Sprint(value),
		},
	})
	return
}

func (dev *Device) Execute(name string, args map[string]interface{}) (debug []string, err error) {
	cmd := packet{
		Cmd:  name,
		Args: map[string]string{},
	}
	for k, v := range args {
		cmd.Args[k] = fmt.Sprint(v)
	}
	var res []packet
	res, err = dev.synchronousWrite(cmd)
	for _, v := range res {
		if v.Cmd == "debug" {
			debug = append(debug, v.Args["msg"])
		}
	}
	return
}

func (dev *Device) Info() (info Info, err error) {
	dev.exec(func() {
		var res []packet
		res, err = dev.write(packet{
			Cmd: "info",
		})
		if err != nil {
			return
		} else if len(res) <= 0 {
			err = errors.New("unexpected info response length")
		} else {
			info.ID = res[0].Args["id"]
			info.Name = res[0].Args["name"]
			info.Model = res[0].Args["model"]
			info.HardwareVer, err = parseVersion(res[0].Args["hw"])
			if err != nil {
				return
			}
			info.FirmwareVer, err = parseVersion(res[0].Args["ver"])
			if err != nil {
				return
			}
			for k, v := range dev.metadata {
				info.Metadata = append(info.Metadata, Metadata{Key: k, Value: v})
			}
		}
	})
	return
}

func (dev *Device) SetOnDisconnect(name string, value interface{}) (err error) {
	_, err = dev.synchronousWrite(packet{
		Cmd: "set",
		Args: map[string]string{
			"name":       name,
			"value":      fmt.Sprint(value),
			"disconnect": fmt.Sprint(true),
		},
	})
	return
}

func (dev *Device) reader() {
	var line string
	var err error

	defer func() {
		dev.connected = false
		dev.subscriptionsSetup = false
		for _, sub := range dev.subscriptions {
			sub.Close()
		}
		dev.Log.Println("[DEVICE] disconnected", err)
	}()

	dev.conn, err = dev.dialer()
	if err != nil {
		return
	}

	reader := bufio.NewReader(dev.conn)
	dev.connected = true
	dev.Log.Println("[DEVICE] connected")

	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@") {
			dev.inbound <- line
		} else {
			dev.Log.Println("[DEVICE] async:", line)
			cmd, err := decode(line)
			dev.Log.Println(cmd.Cmd)
			if err == nil {
				switch cmd.Cmd {
				case "@attr":
					dev.Log.Printf("fanout %+v", cmd)
					dev.fanout(cmd.Args["name"], dev.subscriptions, Message{
						Message: cmd.Args["name"],
						Value:   cmd.Args["value"],
					})
				}
			} else {
				dev.Log.Println("err decoding aync packet:", line)
			}
		}
	}
}

func (dev *Device) execHandler() {
	for {
		select {
		case fn := <-dev.execCh:
			fn()
		case <-time.After(5 * time.Second):
			_, _ = dev.write(packet{Cmd: "ping"})
		}
	}
}

func (dev *Device) exec(fn func()) {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	dev.execCh <- func() {
		fn()
		wg.Done()
	}
	wg.Wait()
}

func (dev *Device) write(cmd packet) (res []packet, err error) {
	if !dev.connected {
		err = ErrNotConnected
		return
	}
	_, err = fmt.Fprintln(dev.conn, encode(cmd))
	for {
		select {

		case line := <-dev.inbound:
			var p packet
			p, err = decode(line)
			if err != nil {
				return
			}
			switch p.Cmd {
			case "ok":
				return
			case "err":
				err = errors.New(p.Args["msg"])
				return
			default:
				res = append(res, p)
			}
		case <-time.After(1 * time.Second):
			err = errors.New("timeout awaiting response")
			dev.conn.Close()
			return
		}
	}
}

func (dev *Device) fanout(key string, subs []*Subscription, transmuted Message) {
	slowSubscribers := make([]*Subscription, 0, len(dev.subscriptions))
	for _, sub := range subs {
		if KeyMatch(key, sub.filter) {
			select {
			case sub.ch <- transmuted:
			default:
				slowSubscribers = append(slowSubscribers, sub)
			}
		}
	}
	for _, sub := range slowSubscribers {
		sub.Close()
		dev.Log.Println("closing slow subscriber", sub)
	}
}

func (dev *Device) Subscribe(filter string) *Subscription {
	sub := &Subscription{
		device: dev,
		filter: filter,
		ch:     make(chan Message, 10),
	}
	dev.execCh <- func() {
		if !dev.subscriptionsSetup {
			dev.Log.Println("enable subscriptions")
			dev.subscriptionsSetup = true
			if _, err := dev.write(packet{Cmd: "sub", Args: map[string]string{"filter": "*"}}); err != nil {
				dev.Log.Println(err)
			}
		}
		dev.subscriptions = append(dev.subscriptions, sub)
	}
	return sub
}
