package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var ErrNotConnected = errors.New("not connected")
var ErrManualDisconnect = errors.New("manual disconnect")

func New(conn io.ReadWriteCloser) *Device {
	dev := Device{
		execCh:    make(chan func()),
		inboundCh: make(chan string),
		asyncCh:   make(chan string, 10), // just in case we have async data in the pipe we hae not read, then wrote data and are waiting for a response
		errCh:     make(chan error),
		conn:      conn,
		runningWG: new(sync.WaitGroup), // this WG is completed when this device is done running and is ready to be cleaned up
		cleanedWG: new(sync.WaitGroup), // this WG is completed when this device is cleaned up
		Log:       log.New(ioutil.Discard, "", log.LstdFlags),
	}
	dev.cleanedWG.Add(1)

	dev.runningWG.Add(1)
	go dev.handleReader()

	dev.runningWG.Add(1)
	go dev.handleExecutor()

	go func() {
		dev.runningWG.Wait()
		dev.cleanup()
	}()

	return &dev
}

type Device struct {
	Log       *log.Logger
	conn      io.ReadWriteCloser
	execCh    chan func()
	inboundCh chan string
	asyncCh   chan string
	errCh     chan error
	err       error
	runningWG *sync.WaitGroup
	cleanedWG *sync.WaitGroup
}

func (d *Device) Write(cmd string) (res []string, err error) {
	if d.err != nil {
		return nil, err

	}
	if strings.TrimSpace(cmd) == "" {
		return nil, errors.New("cmd cannot be empty")
	}
	execErr := d.exec(func() {
		res, err = d.execute(cmd)
	})
	if execErr != nil {
		err = execErr
	}
	return
}

func (d *Device) exec(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrNotConnected
		}
	}()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	//TODO this might hang if the handleExecutor thread goes down, I need to mitigate this somehow

	d.execCh <- func() {
		fn()
		wg.Done()
	}
	wg.Wait()
	return nil
}

func (d *Device) Wait() error {
	d.cleanedWG.Wait()
	return d.err
}

func (d *Device) Disconnect() {
	d.errCh <- ErrManualDisconnect
	_ = d.Wait()
}

func (d *Device) execute(cmd string) (res []string, err error) {
	d.Log.Println("[EXEC ]", cmd)
	_, err = fmt.Fprintln(d.conn, cmd)
	for err == nil {
		select {
		case line := <-d.inboundCh:
			switch line {
			case "ok":
				return
			case "err":
				err = errors.New(line)
			default:
				res = append(res, line)
			}
		case <-time.After(1 * time.Second):
			err = fmt.Errorf("timeout waiting for response line=\"%s\"", cmd)
			d.err = ErrNotConnected
		}
	}
	return
}

func (d *Device) cleanup() {
	d.Log.Println("cleanup")
	defer func() {
		d.cleanedWG.Done()
		d.Log.Println("done")
	}()
	close(d.asyncCh)
	close(d.inboundCh)
	close(d.execCh)
	close(d.errCh)
}

func (d *Device) handleReader() {
	d.Log.Println("[READ ] start")
	defer func() {
		d.Log.Println("[READ ] stop")
		d.runningWG.Done()
	}()
	buf := bufio.NewReader(d.conn)
	for {
		if line, readErr := buf.ReadString('\n'); readErr == nil {
			line = strings.TrimSpace(line)
			if d.err != nil {
				return
			}
			d.Log.Println("[READ ] read:", line)
			if strings.HasPrefix(line, "@") {
				d.asyncCh <- line
			} else {
				d.inboundCh <- line
			}
		} else {
			d.Log.Println("[READ ] error:", readErr)
			select {
			case d.errCh <- readErr:
			default:
			}
			return
		}
	}
}

func (d *Device) handleExecutor() {
	d.Log.Println("[EXEC ] start")
	defer func() {
		d.Log.Println("[EXEC ] stop:", d.err)
		d.conn.Close()
		d.runningWG.Done()
		d.Log.Println("[EXEC ] done")
	}()
	for d.err == nil {
		select {
		case fn := <-d.execCh:
			fn()
		case line := <-d.asyncCh:
			d.handleAsyncData(line)
		case <-time.After(5 * time.Second):
			if _, err := d.execute("ping"); err != nil {
				d.err = ErrNotConnected
			}
		case d.err = <-d.errCh:
		}
	}
}

func (d *Device) handleAsyncData(line string) {
	// TODO handle this send it to subscribers and such
	d.Log.Println("[ASYNC]", line)
}

func main() {
	log.SetPrefix("[MAIN  ]")
	var dev *Device
	go func() {
		// dial wait reconnect loop
		for {
			if conn, err := net.Dial("tcp", "192.168.1.100:5000"); err == nil {
				log.Println("connect")
				dev = New(conn)
				dev.Log = log.New(os.Stdout, "[DEVICE] ", log.LstdFlags)

				// TODfO: something is weird here, if we get an error back from Write we probably need a way of knowing if it is a terminal error
				// Like what if you sent a bad command, and got an error back vs you tried to write and the device was broken

				// turn on the onboard LED, probably should check the errors...
				fmt.Println(dev.Write("set name:led.0 value:true"))
				// set up the last will and testament to turn off the onboard LED, probably should check the errors...
				fmt.Println(dev.Write("set name:led.0 value:false disconnect:true"))
				// subscribe to all events, because why not, probably should check the errors...
				fmt.Println(dev.Write("sub filter:*"))

				if err := dev.Wait(); err != nil {
					log.Println("error:", err)
				}
				// make sure the conn is closed
				_ = conn.Close()
				log.Println("disconnect")
			} else {
				log.Println("error dialing:", err)
			}
		}
	}()

	buff := bufio.NewReader(os.Stdin)
	// read data, then write it, and dump the response
	for {
		line, _ := buff.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "close" {
			dev.Disconnect()
		} else {
			if res, err := dev.Write(line); err == nil {
				for _, r := range res {
					log.Println(r)
				}
			} else {
				log.Println("err:", err)
			}
		}
	}
}
