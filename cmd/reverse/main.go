package main

import (
	"bufio"
	"fmt"
	"github.com/pborges/iotfwdrv"
	"io"
	"log"
	"net"
	"os"
	"time"
)

// this is to help reverse engineer future devices
func main() {
	dev := iotfwdrv.New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", "192.168.1.244:5000", 2*time.Second)
	})
	dev.Log.SetOutput(os.Stdout)

	for {
		if err := dev.Connect(); err == nil {
			go work(dev)
			dev.Wait()
		} else {
			log.Println("error connecting", err)
		}
	}
}

func work(dev *iotfwdrv.Device) {
	pins := []int{
		0,
		//1,
		2,
		//3,
		//4,
		//5,
		//6,
		//7,
		//8,
		9,
		10,
		//11,
		//12,
		13,
		//14,
		15, // perhaps triac?
		16,
	}
	for {
		fmt.Print("scan?")
		//states := make([]string, len(pins))
		bufio.NewReader(os.Stdin).ReadLine()
		for _, v := range pins {
			res, err := dev.Execute("readpin", map[string]interface{}{"pin": v, "up": true})
			if err == nil {
				fmt.Printf("%02d %s\n", v, res.Debug[0])
				//states[k] = res.Debug[0]
				//if res.Debug[0] == "true" {
				//	states[k] = res.Debug[0]
				//} else {
				//	states[k] = false
				//}
			} else {
				fmt.Print("err:", err)
			}
		}
		fmt.Println()
		//for _, v := range pins {
		//	fmt.Printf("%02d ", v)
		//}
		//fmt.Println()
		//for k := range pins {
		//	if states[k] {
		//		fmt.Print(" T ")
		//	} else {
		//		fmt.Print(" F ")
		//	}
		//}
	}
}
