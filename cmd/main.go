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

	dev := iotfwdrv.Device{}
	dev.OnConnect(func() {
		res, err := dev.Exec(iotfwdrv.Packet{Name: "list"})
		if err != nil {
			log.Println(err)
		} else {
			for _, r := range res {
				log.Println(r)
			}
		}

		go func() {
			for i := 0; dev.Connected(); i++ {
				res, err := dev.Exec(iotfwdrv.Packet{Name: "set", Args: map[string]interface{}{"name": "gpio.0", "value": i%2 > 0}})
				if err != nil {
					log.Println("err in exec:", err)
				} else {
					for _, r := range res {
						log.Println(r)
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			log.Println("disconnect")
		}()
	})

	var err error
	for {
		log.Println("dial")
		dev.Conn, err = net.Dial("tcp", "192.168.1.100:5000")
		if err != nil {
			log.Println(err)
			return
		}
		fmt.Println("handler:", dev.Handle())
	}
}
