package main

import (
	"fmt"
	"github.com/pborges/iotfwdrv"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func NewLogToSheet(credentialsFile string, sheetId string) (*Log, error) {
	ctx := context.Background()
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFile), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		return nil, err
	}
	return &Log{
		sheetID: sheetId,
		service: srv,
	}, nil
}

type Log struct {
	sheetID string
	service *sheets.Service
}

func (l Log) Append(range_ string, values ...interface{}) error {
	_, err := l.service.Spreadsheets.Values.Append(l.sheetID, range_, &sheets.ValueRange{
		Values: [][]interface{}{values},
	}).ValueInputOption("RAW").Do()
	return err
}

func main() {
	var dev iotfwdrv.Device

	log2Sheet, err := NewLogToSheet("logtosheet-1601418471376-2733e2db290e.json", "1wXF5dmuEjgjteTWZg0eRz72SBXU4PtHvt5naQBXR250")
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
