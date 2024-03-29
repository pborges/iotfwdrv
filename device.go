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
	Device Metadata
	Key    string
	Value  string
}

type Device struct {
	Log           *log.Logger
	VerboseLog    bool
	info          Metadata
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
	lastRead      time.Time
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
			dev.Log.Println("disconnect called manually")
			err = dev.conn.Close()
			err = fmt.Errorf("manual disconnect err: %w", err)
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
			dev.connected = false
			if dev.conn != nil {
				dev.conn.Close()
			}
			return
		}
		dev.Log.SetPrefix("[" + dev.Info().ID + ":" + dev.Info().Name + "] ")

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
		dev.info.FirmwareVer, err = ParseVersion(res[0].Args["fw"])
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

type Response struct {
	Output []string
	Debug  []string
}

func (dev *Device) Execute(name string, args map[string]interface{}) (res Response, err error) {
	cmd := packet{
		Cmd:  name,
		Args: map[string]string{},
	}
	if args != nil {
		for k, v := range args {
			cmd.Args[k] = fmt.Sprint(v)
		}
	}
	var r []packet
	r, err = dev.synchronousWrite(cmd)
	for _, v := range r {
		if v.Cmd == "output" {
			res.Output = append(res.Output, v.Args["msg"])
		} else if v.Cmd == "debug" {
			res.Debug = append(res.Debug, v.Args["msg"])
		}
	}
	return
}

func (dev *Device) Info() Metadata {
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
			dev.exec(func() {
				dev.connected = false
				// close all subscriptions
				for _, sub := range dev.subscriptions {
					// dont call sub.Close() while iterating
					close(sub.ch)
				}
				dev.subscriptions = make([]*Subscription, 0)
				for _, w := range dev.waiting {
					w <- err
					close(w)
				}
				dev.waiting = make([]chan error, 0)
				dev.Log.Println("disconnected", err)
			})
		}()

		for {
			line, err = reader.ReadString('\n')
			if err != nil {
				err = fmt.Errorf("unable to read %w", err)
				return
			}
			dev.lastRead = time.Now()
			line = strings.TrimSpace(line)
			if dev.VerboseLog {
				dev.Log.Println("read:", line)
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "@") {
				dev.inbound <- line
			} else {
				cmd, err := decode(line)
				if err == nil {
					switch cmd.Cmd {
					case "@attr":
						dev.valuesLock.Lock()
						dev.values[cmd.Args["name"]] = cmd.Args["value"]
						if cmd.Args["name"] == "config.name" {
							info := dev.info
							info.Name = cmd.Args["value"]
							dev.info = info
						}
						dev.valuesLock.Unlock()

						dev.fanout(cmd.Args["name"], cmd.Args["value"])
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
		case <-time.After(1 * time.Second):
			if time.Since(dev.lastRead) > 10*time.Second {
				_, _ = dev.write(packet{Cmd: "ping"})
			}
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
	encoded := encode(cmd)
	if dev.VerboseLog {
		dev.Log.Println("write:", encoded)
	}
	_, err = fmt.Fprintln(dev.conn, encoded)
	if err != nil {
		err = fmt.Errorf("unable to write data %w", err)
		return
	}
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
				err = fmt.Errorf("error from device %s", p.Args["msg"])
				return
			default:
				res = append(res, p)
			}
		case <-time.After(2 * time.Second):
			err = errors.New("timeout awaiting response")
			dev.conn.Close()
			return
		}
	}
}

func (dev *Device) fanout(key string, value string) {
	slowSubscribers := make([]*Subscription, 0, len(dev.subscriptions))
	for _, sub := range dev.subscriptions {
		if KeyMatch(key, sub.filter) {
			select {
			case sub.ch <- Message{
				Device: dev.info,
				Key:    key,
				Value:  value,
			}:
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
	dev.exec(func() {
		sub.execCh = dev.execCh
		dev.subscriptions = append(dev.subscriptions, sub)
	})
	return sub
}
