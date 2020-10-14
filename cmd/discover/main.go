package main

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/pborges/iotfwdrv"
	"log"
	"os"
)

func main() {
	networks, err := iotfwdrv.LocalNetworks()
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Attempting discovery on", networks)
	devs := iotfwdrv.Discover(networks...)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Model", "HW VER", "FW VER", "Addr"})

	for _, dev := range devs {
		table.Append([]string{dev.Info().ID, dev.Info().Name, dev.Info().Model, dev.Info().HardwareVer.String(), dev.Info().FirmwareVer.String(), dev.Addr()})
	}

	table.Render()
}
