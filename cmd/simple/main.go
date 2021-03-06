package main

import (
	"github.com/pborges/iotfwdrv"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	dev := iotfwdrv.New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", "192.168.1.202:5000", 2*time.Second)
	})
	dev.Log.SetOutput(os.Stdout)

	for {
		if err := dev.Connect(); err == nil {
			go subscriber(dev)
			work(dev)
		} else {
			log.Println("error connecting", err)
		}
	}
}

func work(dev *iotfwdrv.Device) {
	var state bool
	var err error

	log.Println("work connect")
	defer log.Println("work disconnect")

	for ; err == nil; err = dev.Set("led.0", state) {
		state = !state
		time.Sleep(100 * time.Millisecond)
	}
}

func subscriber(dev *iotfwdrv.Device) {
	log.Println("sub connect")
	defer log.Println("sub disconnect")

	for msg := range dev.Subscribe(">").Chan() {
		log.Println("GOT SUB", msg)
	}
}
