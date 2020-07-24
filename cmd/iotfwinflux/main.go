package main

import (
	"context"
	influxdb "github.com/influxdata/influxdb-client-go"
	"github.com/influxdata/influxdb-client-go/api"
	"github.com/pborges/iotfwdrv"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	client := influxdb.NewClient("http://stratus:9999", "lX8mrFJkc-gDKfqQC91MEVmwdQln9rfC6fF7xTfqWcDENz67yQIcyhuXXrxpO_foy4WytMLjdxT5FvtBWcflpQ==")
	apiBlocking := client.WriteAPIBlocking("iot", "data")
	defer client.Close()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags)

	connectors := []DeviceConnector{
		{"192.168.1.242:5000", ">", "led.0"}, // OFFICE id:esp-7fe02c
		{"192.168.1.135:5000", ">", "led.0"}, // DOWNSTAIRS id:esp-35ce6a
		{"192.168.1.136:5000", ">", "led.0"}, // OUTSIDE id:esp-7fde02
		{"192.168.1.128:5000", ">", "led.0"}, // S31 id:esp-067599
		{"192.168.1.120:5000", ">", "led.0"}, // WEMOS BE280 id:esp-49b46b
		{"192.168.1.116:5000", ">", "led.0"}, // WEMOS BE280 id:esp-60b180
	}

	for _, c := range connectors {
		go handle(apiBlocking, c)
	}

	select {}
}

func writeData(api api.WriteAPIBlocking, node string, name string, data float64) error {
	p := influxdb.NewPointWithMeasurement(node).
		AddField(name, data).
		SetTime(time.Now())
	return api.WritePoint(context.Background(), p)
}

type DeviceConnector struct {
	Addr     string
	Filter   string
	LastWill string
}

func handle(api api.WriteAPIBlocking, connector DeviceConnector) {
	log.Println("# logging", connector.Filter, "from", connector.Addr, "will", connector.LastWill)
	for {
		if conn, err := net.DialTimeout("tcp", connector.Addr, 2*time.Second); err == nil {
			log.Println("# connected")
			for dev := iotfwdrv.Handler(conn); dev.Connected(); {
				dev.Log.SetOutput(os.Stdout)
				dev.Log.SetPrefix("[" + dev.Id() + " " + conn.RemoteAddr().String() + "] ")

				// do some work
				if connector.LastWill != "" {
					_, err = dev.Exec(iotfwdrv.SetPacket{Key: connector.LastWill, Value: true})
					if err != nil {
						return
					}

					_, err = dev.Exec(iotfwdrv.SetPacket{Key: connector.LastWill, Value: false, Disconnect: true})
					if err != nil {
						return
					}
				}

				sub := dev.Subscribe(connector.Filter)
				for p := range sub.Chan() {
					var err error
					switch cmd := p.(type) {
					case iotfwdrv.BooleanAttributeUpdate:
						var data float64
						if cmd.Value {
							data = 1
						}
						log.Println("INFLUX:", dev.Id(), cmd.Name, data)
						err = writeData(api, dev.Id(), cmd.Name, data)
					case iotfwdrv.IntegerAttributeUpdate:
						log.Println("INFLUX:", dev.Id(), cmd.Name, cmd.Value)
						err = writeData(api, dev.Id(), cmd.Name, float64(cmd.Value))
					case iotfwdrv.UnsignedAttributeUpdate:
						log.Println("INFLUX:", dev.Id(), cmd.Name, cmd.Value)
						err = writeData(api, dev.Id(), cmd.Name, float64(cmd.Value))
					case iotfwdrv.DoubleAttributeUpdate:
						log.Println("INFLUX:", dev.Id(), cmd.Name, cmd.Value)
						err = writeData(api, dev.Id(), cmd.Name, cmd.Value)
					default:
						log.Printf("# unknown %+v", p)
					}
					if err != nil {
						log.Printf("INFLUX ERR: %+v", err)
					}
				}
			}
			log.Println("# disconnected")
		} else {
			log.Println("# reconnecting", err)
			time.Sleep(2 * time.Second)
		}
	}
}
