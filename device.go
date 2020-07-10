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
	C      chan Packet
	device *Device
	filter string
}

func (s *Subscription) Close() {
	close(s.C)
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

func Handler(conn io.ReadWriter) *Device {
	dev := &Device{
		conn:   conn,
		readCh: make(chan string),
		execCh: make(chan func()),
		Log:    log.New(ioutil.Discard, "[iotfwdrv] ", log.LstdFlags),
	}

	dev.executor()
	dev.reader()
	dev.connected = true
	return dev
}

type Device struct {
	Log           *log.Logger
	conn          io.ReadWriter
	readCh        chan string
	execCh        chan func()
	connected     bool
	subscriptions []*Subscription
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
		C:      make(chan Packet),
	}
	d.execCh <- func() {
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
		case <-time.After(1 * time.Second):
			err = errors.New("timeout")
			return
		}
	}
}

func (d *Device) executor() {
	go func() {
		d.Log.Println("executor start")
		defer func() {
			d.Log.Println("executor stop")
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
					if _, err := d.exec(PingPacket{}); err != nil {
						d.connected = false
						return
					}
				}
			}
		}
	}()
}

func (d *Device) handleAsync(cmd rawPacket) {
	switch cmd.Cmd() {
	case "@attr":
		valueUpdate := AttributeUpdatePacket{
			Name: cmd.Get("name").(string),
			Type: cmd.Get("type").(string),
		}
		switch valueUpdate.Type {
		case "bool":
			valueUpdate.Value = cmd.Get("value") == "true"
		case "unsigned":
			valueUpdate.Value, _ = strconv.ParseUint(cmd.Get("value").(string), 10, 64)
		case "integer":
			valueUpdate.Value, _ = strconv.ParseInt(cmd.Get("value").(string), 10, 64)
		case "string":
			valueUpdate.Value, _ = cmd.Get("value").(string)
		default:
			d.Log.Println("unknown @attr type", valueUpdate.Type)
		}
		d.execCh <- func() {
			for _, sub := range d.subscriptions {
				if KeyMatch(cmd.Get("value").(string), sub.filter) {
					go func(sub *Subscription, cmd Packet) {
						select {
						case sub.C <- cmd:
						case <-time.After(5 * time.Second):
							d.Log.Println("timeout fanning out subscription to", sub.filter)
						}
					}(sub, valueUpdate)
				}
			}
		}
	default:
		d.Log.Println("unhandled async command,", cmd.Cmd())
	}
}

func (d *Device) reader() {
	scanner := bufio.NewScanner(d.conn)
	go func() {
		d.Log.Println("reader start")
		defer func() {
			d.Log.Println("reader stop")
			d.disconnect()
		}()
		for scanner.Scan() {
			msg := scanner.Text()
			if strings.HasPrefix(msg, "@") {
				if cmd, err := Decode(msg); err == nil {
					d.handleAsync(cmd)
				} else {
					d.Log.Println("unable to decode async message", err)
				}
			} else {
				d.Log.Println(">", msg)
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
	for _, sub := range d.subscriptions {
		close(sub.C)
	}
}
