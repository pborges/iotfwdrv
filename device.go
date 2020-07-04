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
	return dev
}

type Device struct {
	conn      io.ReadWriteCloser
	readCh    chan string
	execCh    chan func()
	connected bool
	Log       *log.Logger
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

func (d *Device) exec(cmd Packet) (res []Packet, err error) {
	encoded := Encode(cmd)
	d.Log.Println(">", encoded)
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

func (d *Device) handleAsync(cmd Packet) {

}

func (d *Device) reader() {
	scanner := bufio.NewScanner(d.conn)
	go func() {
		d.Log.Println("reader start")
		defer func() {
			d.Log.Println("reader stop")
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
				d.Log.Println("<", msg)
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
