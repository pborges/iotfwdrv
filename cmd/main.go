package main

import (
	"fmt"
	iotfwdrv2 "github.com/pborges/iotfwdrv"
	"github.com/pborges/iotfwdrv/logtosheet"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	var dev iotfwdrv2.Device

	log2Sheet, err := logtosheet.New("logtosheet/logtosheet-1601418471376-2733e2db290e.json", "1wXF5dmuEjgjteTWZg0eRz72SBXU4PtHvt5naQBXR250")
	if err != nil {
		panic(err)
	}

	dev.Log = log.New(os.Stdout, "[iotfwdrv] ", log.LstdFlags)

	// once started this device will attempt to keep in connection with the endpoint by calling the dialer when needed
	if err := dev.Start(func() (io.ReadWriteCloser, error) {
		var addr = "192.168.1.100:5000"
		dev.SetMetadata("addr", addr)
		return net.DialTimeout("tcp", addr, 2*time.Second)
	}); err != nil {
		log.Println(err)
	}

	// verify you can only "start" once
	if err := dev.Start(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", "192.168.1.100:5000", 2*time.Second)
	}); err != nil {
		log.Println(err)
	}
	for {
		if !dev.Connected() {
			continue
		}
		info, err := dev.Info()
		if err != nil {
			log.Println("err:", err)
			continue
		}
		fmt.Printf("%+v\n", info)

		if err := dev.Set("led.0", true); err != nil {
			log.Println("err:", err)
			continue
		}
		if err := dev.SetOnDisconnect("led.0", false); err != nil {
			log.Println("err:", err)
			continue
		}

		for val := range dev.Subscribe("gpio.0").Chan() {
			if val.Value == "true" {
				log.Println("attempting to write to log2sheet")
				if err := log2Sheet.Append("A:A", time.Now().Format(time.Stamp)); err != nil {
					log.Println("err:", err)
					continue
				}
				if err := dev.Set("gpio.0", false); err != nil {
					log.Println("err:", err)
					continue
				}
			}
		}
		log.Println("subscription closed")

		//var state bool
		//for err == nil {
		//	err = dev.Set("gpio.0", state)
		//	state = !state
		//	time.Sleep(100 * time.Millisecond)
		//}
		//log.Println("err:", err)
	}
}
