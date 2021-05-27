package main

import (
	"bufio"
	"fmt"
	"github.com/pborges/iotfwdrv"
	"log"
	"os"
	"time"
)

func main() {
	Logger := log.New(os.Stdout, "", log.LstdFlags)
	svc := iotfwdrv.Service{
		Log: Logger,
		OnConnect: func(ctx iotfwdrv.DeviceContext) {
			//ctx.Log = Logger
			//ctx.VerboseLog = true

			if ctx.Get("led.0") != "" {
				if err := ctx.Set("led.0", true); err != nil {
					Logger.Printf("unable to set %s:led.0 to true", ctx.Info().ID)
				}
				if err := ctx.SetOnDisconnect("led.0", false); err != nil {
					Logger.Printf("unable to set %s:led.0 to false on disconnect", ctx.Info().ID)
				}
			}
		},
	}

	go func() {
		for m := range svc.Subscribe("*.@event").Chan() {
			fmt.Println("RSSI: ("+m.Device.Name+")", m.Key, m.Value)
		}
	}()

	fnCh := make(chan func())
	go func() {
		Logger.Printf("discover complete err:%+v\n", svc.Discover())
		time.Sleep(1 * time.Second)
		svc.RenderDevicesTable(os.Stdout)
		for {
			select {
			case fn := <-fnCh:
				fn()
			case <-time.After(5 * time.Second):
				svc.RenderDevicesTable(os.Stdout)
				Logger.Println("press enter to discover again")
			}
		}
	}()

	for {
		bufio.NewReader(os.Stdin).ReadLine()
		fnCh <- func() {
			Logger.Printf("discover complete err:%+v\n", svc.Discover())
		}
	}
}
