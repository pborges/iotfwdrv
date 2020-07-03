package iotfwdrv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type Device struct {
	Conn         io.ReadWriteCloser
	onConnect    func()
	onDisconnect func()
	readCh       chan string
	execCh       chan func()
	err          error
}

func (d *Device) OnConnect(fn func()) {
	d.onConnect = fn
	if d.Connected() {
		d.onConnect()
	}
}

func (d *Device) OnDisconnect(fn func()) {
	d.onDisconnect = fn
	if !d.Connected() {
		d.onDisconnect()
	}
}

func (d Device) Connected() bool {
	return d.execCh != nil
}

func (d *Device) Handle() error {
	if d.execCh != nil {
		return errors.New("already running")
	}
	d.execCh = make(chan func())
	d.readCh = make(chan string)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer func() {
			wg.Done()
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
				if _, err := d.exec(Packet{Name: "ping"}); err != nil {
					d.err = err
					return
				}
			}
		}
	}()
	d.execCh <- func() {
		d.reader(wg)
	}
	if d.onConnect != nil {
		d.onConnect()
	}
	wg.Wait()
	d.readCh = nil
	d.Conn = nil
	err := d.err
	d.err = nil
	d.execCh = nil
	if d.onDisconnect != nil {
		d.onDisconnect()
	}
	return err
}

func (d *Device) isClosed() bool {
	select {
	case <-d.execCh:
		return true
	default:
	}
	return false
}

func (d *Device) Exec(cmd Packet) (res []Packet, err error) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	defer func() {
		if recover() != nil {
			err = errors.New("not connected")
		}
	}()

	d.execCh <- func() {
		res, err = d.exec(cmd)
		wg.Done()
	}
	wg.Wait()
	return
}

func (d *Device) exec(cmd Packet) (res []Packet, err error) {
	if _, err = fmt.Fprintln(d.Conn, Encode(cmd)); err != nil {
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

func (d *Device) reader(wg *sync.WaitGroup) {
	scanner := bufio.NewScanner(d.Conn)
	go func() {
		defer func() {
			wg.Done()
		}()
		for scanner.Scan() {
			msg := scanner.Text()
			if strings.HasPrefix(msg, "@") {
				fmt.Println(msg)
			} else {
				d.readCh <- msg
			}
		}
		if err := scanner.Err(); err != nil {
			d.err = err
			close(d.execCh)
		}
		close(d.readCh)
	}()
}
