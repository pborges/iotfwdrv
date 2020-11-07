package iotfwdrv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

var ErrNotConnected = errors.New("not connected")

func New(dialer func() (io.ReadWriteCloser, error)) *Device {
	var dev Device

	dev.execCh = make(chan func())
	dev.inbound = make(chan string)

	dev.values = make(map[string]string)
	dev.dialer = dialer
	dev.Log = log.New(ioutil.Discard, "[iotfwdrv] ", log.LstdFlags)

	go dev.execHandler()
	return &dev
}

type Message struct {
	Message string
	Value   string
}

type Info struct {
	ID          string
	Name        string
	Model       string
	FirmwareVer Version
	HardwareVer Version
}

type Device struct {
	Log           *log.Logger
	VerboseLog    bool
	info          Info
	connected     bool
	dialer        func() (io.ReadWriteCloser, error)
	execCh        chan func()
	inbound       chan string
	conn          io.ReadWriteCloser
	setup         sync.Once
	subscriptions []*Subscription
	values        map[string]string
	valuesLock    sync.Mutex
	waiting       []chan error
}

func (dev *Device) Wait() error {
	var c chan error
	dev.exec(func() {
		if dev.connected {
			c = make(chan error)
			dev.waiting = append(dev.waiting, c)
		}
	})
	if c != nil {
		return <-c
	}
	return ErrNotConnected
}

func (dev *Device) Connected() bool {
	return dev.connected
}

func (dev *Device) Addr() *net.TCPAddr {
	if dev.conn != nil {
		if c, ok := dev.conn.(*net.TCPConn); ok {
			return c.RemoteAddr().(*net.TCPAddr)
		}
	}
	return nil
}

func (dev *Device) Disconnect() (err error) {
	dev.exec(func() {
		if dev.conn != nil {
			dev.conn.Close()
		}
	})
	return err
}

func (dev *Device) Connect() (err error) {
	dev.exec(func() {
		if dev.connected {
			return
		}

		dev.conn, err = dev.dialer()
		if err != nil {
			return
		}
		dev.reader()

		// get the info packet
		if err = dev.getInfo(); err != nil {
			return
		}
		dev.Log.SetPrefix("[" + dev.info.ID + "] ")

		// subscribe to all
		if _, err = dev.write(packet{Cmd: "sub", Args: map[string]string{"filter": "*"}}); err != nil {
			dev.Log.Println("subscriptions not supported")
			err = nil
		}
		dev.Log.Println("connected")
	})
	return
}

func (dev *Device) getInfo() (err error) {
	var res []packet
	res, err = dev.write(packet{
		Cmd: "info",
	})
	if err != nil {
		return
	} else if len(res) <= 0 {
		err = errors.New("unexpected info response length")
	} else {
		dev.info.ID = res[0].Args["id"]
		dev.info.Model = res[0].Args["model"]
		dev.info.HardwareVer, err = ParseVersion(res[0].Args["hw"])
		if err != nil {
			return
		}
		dev.info.FirmwareVer, err = ParseVersion(res[0].Args["ver"])
		if err != nil {
			return
		}
	}

	res, err = dev.write(packet{
		Cmd: "list",
	})
	if err != nil {
		return
	} else if len(res) <= 0 {
		err = errors.New("unexpected list response length")
	} else {
		for _, p := range res {
			switch p.Cmd {
			case "attr":
				if p.Args["name"] == "config.name" {
					dev.info.Name = p.Args["value"]
				}
				dev.valuesLock.Lock()
				dev.values[p.Args["name"]] = p.Args["value"]
				dev.valuesLock.Unlock()
			}
		}
	}
	return
}

func (dev *Device) synchronousWrite(cmd packet) (res []packet, err error) {
	dev.exec(func() {
		res, err = dev.write(cmd)
	})
	return
}

func (dev *Device) SetName(value string) (err error) {
	if err := dev.Set("config.name", value); err == nil {
		dev.info.Name = value
	}
	return err
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

func (dev *Device) Get(attr string) (val string) {
	dev.valuesLock.Lock()
	defer dev.valuesLock.Unlock()
	val = dev.values[attr]
	return
}

func (dev *Device) Execute(name string, args map[string]interface{}) (output []string, err error) {
	cmd := packet{
		Cmd:  name,
		Args: map[string]string{},
	}
	if args != nil {
		for k, v := range args {
			cmd.Args[k] = fmt.Sprint(v)
		}
	}
	var res []packet
	res, err = dev.synchronousWrite(cmd)
	for _, v := range res {
		if v.Cmd == "output" {
			output = append(output, v.Args["msg"])
		}
	}
	return
}

func (dev *Device) Info() Info {
	return dev.info
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

	reader := bufio.NewReader(dev.conn)
	dev.connected = true
	go func() {
		defer func() {
			dev.connected = false
			for _, sub := range dev.subscriptions {
				sub.Close()
			}
			for _, w := range dev.waiting {
				w <- err
				close(w)
			}
			dev.waiting = make([]chan error, 0)
			dev.Log.Println("disconnected", err)
		}()

		for {
			line, err = reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "@") {
				dev.inbound <- line
			} else {
				if dev.VerboseLog {
					dev.Log.Println("async:", line)
				}
				cmd, err := decode(line)
				if err == nil {
					switch cmd.Cmd {
					case "@attr":
						dev.valuesLock.Lock()
						dev.values[cmd.Args["name"]] = cmd.Args["value"]
						dev.valuesLock.Unlock()

						if dev.VerboseLog {
							dev.Log.Printf("fanout %+v", cmd)
						}
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
	}()
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
		dev.subscriptions = append(dev.subscriptions, sub)
	}
	return sub
}
