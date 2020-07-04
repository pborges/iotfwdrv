package main

import (
	"fmt"
	"github.com/pborges/iotfwdrv"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Llongfile)
	for {
		if conn, err := net.DialTimeout("tcp", "192.168.1.100:5000", 2*time.Second); err == nil {
			fmt.Println("connected")
			for dev := iotfwdrv.Handler(conn); dev.Connected(); {
				dev.Log.SetOutput(os.Stdout)
				dev.Log.SetPrefix("[" + conn.RemoteAddr().String() + "] ")
				if err := work(dev); err != nil {
					fmt.Println("error working", err)
				}
			}
			fmt.Println("disconnected")
		} else {
			fmt.Println("reconnecting", err)
			time.Sleep(2 * time.Second)
		}
	}
}

func work(dev *iotfwdrv.Device) (err error) {
	_, err = dev.Exec(iotfwdrv.Packet{Name: "set", Args: map[string]interface{}{"name": "gpio.0", "value": true}})
	if err != nil {
		return
	}

	_, err = dev.Exec(iotfwdrv.Packet{Name: "set", Args: map[string]interface{}{"name": "gpio.0", "value": false, "disconnect": true}})
	if err != nil {
		return
	}

	for i := 0; ; i++ {
		_, err = dev.Exec(iotfwdrv.Packet{Name: "set", Args: map[string]interface{}{"name": "gpio.1", "value": i%2 > 0}})
		if err != nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
