package main

import (
	"bufio"
	"fmt"
	"github.com/pborges/iotfwdrv"
	"log"
	"os"
)

func main() {
	Logger := log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)
	svc := iotfwdrv.Service{
		Log: Logger,
		OnConnect: func(ctx iotfwdrv.DeviceContext) {
			ctx.Log = Logger
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
	go svc.HandleMDNS()

	go func() {
		for m := range svc.Subscribe("*.@event").Chan() {
			fmt.Printf("%s (%s) %s %s\n", m.Device.ID, m.Device.Name, m.Key, m.Value)
		}
	}()

	svc.OnRegister = func(ctx iotfwdrv.MetadataAndAddr) {
		fmt.Printf("Discovered %+v\n", ctx)
	}

	fnCh := make(chan func())

	for {
		bufio.NewReader(os.Stdin).ReadLine()
		fnCh <- func() {
			svc.RenderDevicesTable(os.Stdout)
		}
	}
}
