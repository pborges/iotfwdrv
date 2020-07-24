package main

import (
	"encoding/csv"
	"fmt"
	"github.com/pborges/iotfwdrv"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags)

	if len(os.Args) < 3 {
		log.Println("usage:", os.Args[0], "<addr> <filter>")
		return
	}

	addr := os.Args[1]
	filter := os.Args[2]
	output := os.Args[3]
	var lastWill string
	if len(os.Args) >= 3 {
		lastWill = os.Args[4]
	}

	log.Println("# logging", filter, "from", addr, "to", output, "will", lastWill)
	fs, _ := os.OpenFile(output, os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0755)
	w := csv.NewWriter(fs)
	w.Write([]string{
		"Timestamp",
		"Name",
		"Value",
	})
	w.Flush()
	for {
		if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
			log.Println("# connected")
			for dev := iotfwdrv.Handler(conn); dev.Connected(); {
				dev.Log.SetOutput(os.Stdout)
				dev.Log.SetPrefix("[" + conn.RemoteAddr().String() + "] ")

				// do some work
				if lastWill != "" {
					_, err = dev.Exec(iotfwdrv.SetPacket{Key: lastWill, Value: true})
					if err != nil {
						return
					}

					_, err = dev.Exec(iotfwdrv.SetPacket{Key: lastWill, Value: false, Disconnect: true})
					if err != nil {
						return
					}
				}

				sub := dev.Subscribe(filter)
				for p := range sub.Chan() {
					switch cmd := p.(type) {
					case iotfwdrv.BooleanAttributeUpdate:
						log.Println(cmd.Name, cmd.Value)
						w.Write([]string{
							time.Now().Format(time.RFC3339),
							cmd.Name,
							fmt.Sprint(cmd.Value),
						})
					case iotfwdrv.IntegerAttributeUpdate:
						log.Println(cmd.Name, cmd.Value)
						w.Write([]string{
							time.Now().Format(time.RFC3339),
							cmd.Name,
							fmt.Sprint(cmd.Value),
						})
					case iotfwdrv.UnsignedAttributeUpdate:
						log.Println(cmd.Name, cmd.Value)
						w.Write([]string{
							time.Now().Format(time.RFC3339),
							cmd.Name,
							fmt.Sprint(cmd.Value),
						})
					case iotfwdrv.DoubleAttributeUpdate:
						log.Println(cmd.Name, cmd.Value)
						w.Write([]string{
							time.Now().Format(time.RFC3339),
							cmd.Name,
							fmt.Sprint(cmd.Value),
						})
					default:
						log.Println("# unknown", p)
					}
					w.Flush()
				}
			}
			log.Println("# disconnected")
		} else {
			log.Println("# reconnecting", err)
			time.Sleep(2 * time.Second)
		}
	}
}
