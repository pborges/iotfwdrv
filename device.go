package iotfwdrv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Subscription struct {
	ch     chan Packet
	device *Device
	filter string
}

func (s *Subscription) Chan() <-chan Packet {
	return s.ch
}

func (s *Subscription) Close() {
	close(s.ch)
	s.device.execCh <- func() {
		for i, sub := range s.device.subscriptions {
			if s == sub {
				s.device.subscriptions[i] = s.device.subscriptions[len(s.device.subscriptions)-1] // Copy last element to index i.
				s.device.subscriptions[len(s.device.subscriptions)-1] = nil                       // Erase last element (write zero value).
				s.device.subscriptions = s.device.subscriptions[:len(s.device.subscriptions)-1]   // Truncate slice.
			}
		}
	}
}

func Handler(conn io.ReadWriteCloser) *Device {
	dev := &Device{
		conn:   conn,
		readCh: make(chan string),
		execCh: make(chan func()),
		Log:    log.New(ioutil.Discard, "[iotfwdrv] ", log.LstdFlags),
	}

	dev.executor()
	dev.reader()
	dev.connected = true

	// TODO fix this hacky BS
	res, _ := dev.exec(InfoPacket{})
	dev.info = res[0].(rawPacket)
	return dev
}

type Device struct {
	Log           *log.Logger
	FlushFn       func() error
	conn          io.ReadWriteCloser
	readCh        chan string
	execCh        chan func()
	connected     bool
	subscriptions []*Subscription
	subscribed    bool

	info rawPacket
}

func (d *Device) Connected() bool {
	return d.connected
}

func (d *Device) Exec(cmd Packet) (res []Packet, err error) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	defer func() {
		if recover() != nil {
			err = errors.New("not connected")
		}
		if err != nil {
			d.connected = false
		}
	}()

	d.execCh <- func() {
		res, err = d.exec(cmd)
		wg.Done()
	}
	wg.Wait()
	return
}

func (d *Device) Subscribe(filter string) *Subscription {
	sub := &Subscription{
		device: d,
		filter: filter,
		ch:     make(chan Packet, 10),
	}
	d.execCh <- func() {
		if !d.subscribed {
			if _, err := d.exec(SubscribePacket{Filter: "*"}); err != nil {
				fmt.Println(err)
			}
		}
		d.subscriptions = append(d.subscriptions, sub)
	}
	return sub
}

func (d *Device) exec(cmd Packet) (res []Packet, err error) {
	encoded := Encode(cmd)
	d.Log.Println("<", encoded)
	if _, err = fmt.Fprintln(d.conn, encoded); err != nil {
		return
	}
	if d.FlushFn != nil {
		if err = d.FlushFn(); err != nil {
			return
		}
	}
	for {
		select {
		case r, ok := <-d.readCh:
			if !ok {
				return
			}
			if r == "ok" {
				return
			} else if strings.HasPrefix(r, "err") {
				err = errors.New(r)
				return
			} else {
				var c Packet
				if c, err = Decode(r); err != nil {
					return
				}
				res = append(res, c)
			}
		case <-time.After(2 * time.Second):
			err = errors.New("timeout")
			return
		}
	}
}

func (d *Device) executor() {
	go func() {
		d.Log.Println("executor start")
		var err error
		defer func() {
			d.Log.Println("executor stop", err)
			d.connected = false
		}()
		for {
			select {
			case fn, ok := <-d.execCh:
				if !ok {
					return
				}
				if fn != nil {
					fn()
				}
			case <-time.After(5 * time.Second):
				if d.Connected() {
					if _, err = d.exec(PingPacket{}); err != nil {
						return
					}
				}
			}
		}
	}()
}

func (d *Device) handleAsync(cmd rawPacket) (err error) {
	switch cmd.Cmd() {
	case "@attr":
		var packet Packet
		tipe := cmd.Get("type").(string)
		switch tipe {
		case "bool":
			val, _ := strconv.ParseBool(cmd.Get("value").(string))
			packet = BooleanAttributeUpdate{
				Name:  cmd.Get("name").(string),
				Value: val,
			}
		case "unsigned":
			val, _ := strconv.ParseUint(cmd.Get("value").(string), 10, 64)
			packet = UnsignedAttributeUpdate{
				Name:  cmd.Get("name").(string),
				Value: val,
			}
		case "int":
			val, _ := strconv.ParseInt(cmd.Get("value").(string), 10, 64)
			packet = IntegerAttributeUpdate{
				Name:  cmd.Get("name").(string),
				Value: val,
			}
		case "double":
			val, _ := strconv.ParseFloat(cmd.Get("value").(string), 64)
			packet = DoubleAttributeUpdate{
				Name:  cmd.Get("name").(string),
				Value: val,
			}
		case "string":
			packet = StringAttributeUpdate{
				Name:  cmd.Get("name").(string),
				Value: cmd.Get("value").(string),
			}
		default:
			d.Log.Println("unknown @attr type", tipe)
		}
		d.Log.Printf("fanout %+v", cmd)
		d.fanout(cmd.Get("name").(string), d.subscriptions, packet)
	default:
		d.Log.Println("unhandled async command,", cmd.Cmd())
	}
	return
}

func (d *Device) fanout(key string, subs []*Subscription, transmuted Packet) {
	slowSubscribers := make([]*Subscription, 0, len(d.subscriptions))
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
		d.Log.Println("closing slow subscriber", sub)
	}
}

func (d *Device) reader() {
	scanner := bufio.NewScanner(d.conn)
	go func() {
		var cmd rawPacket
		var err error
		d.Log.Println("reader start")
		defer func() {
			d.Log.Println("reader stop", err)
			d.disconnect()
		}()
		for scanner.Scan() {
			msg := scanner.Text()
			d.Log.Println(">", msg)
			if strings.HasPrefix(msg, "@") {
				if cmd, err = Decode(msg); err == nil {
					if err = d.handleAsync(cmd); err != nil {
						return
					}
				} else {
					d.Log.Println("unable to decode async message", err)
				}
			} else {
				d.readCh <- msg
			}
		}
		if err := scanner.Err(); err != nil {
			d.connected = false
			close(d.execCh)
		}
		close(d.readCh)
	}()
}

func (d *Device) disconnect() {
	d.Log.Println("disconnect")
	//for _, sub := range d.subscriptions {
	//	close(sub.ch)
	//}
}

func (d *Device) Id() string {
	return d.info.Get("id").(string)
}
