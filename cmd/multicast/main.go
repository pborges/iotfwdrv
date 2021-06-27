package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

const MaxDatagramSize = 256

func main() {
	addr, err := net.ResolveUDPAddr("udp", "226.1.13.37:5000")
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		conn, err := net.ListenMulticastUDP("udp", nil, addr)
		if err != nil {
			log.Fatal(err)
		}

		conn.SetReadBuffer(MaxDatagramSize)
		for {
			b := make([]byte, MaxDatagramSize)
			n, src, err := conn.ReadFromUDP(b)
			if err != nil {
				log.Fatal("ReadFromUDP failed:", err)
			}
			res := string(b[:n])
			if strings.HasPrefix(res, "iotfw found ") {
				fmt.Println(strings.TrimPrefix(res, "iotfw found "), src)
			}
		}
	}()

	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		bufio.NewReader(os.Stdin).ReadLine()
		if _, err := c.Write([]byte("iotfw discover")); err != nil {
			log.Fatal(err)
		}
	}
}
