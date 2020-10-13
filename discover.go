package iotfwdrv

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func parseIp(addr string) (ip [4]int) {
	for i, seg := range strings.Split(addr, ".") {
		ip[i], _ = strconv.Atoi(seg)
	}
	return
}

func Probe(addr string) (*Device, error) {
	dev := new(Device)
	dev.Log = log.New(os.Stdout, "[iotfwdrv] ", log.LstdFlags)
	dialerFn := func() (io.ReadWriteCloser, error) {
		dev.Log.Println("attempting to connect to", addr)
		return net.DialTimeout("tcp", addr, 2*time.Second)
	}
	conn, err := dialerFn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.(*net.TCPConn).SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	_, err = fmt.Fprintln(conn, "info")
	if err != nil {
		return nil, fmt.Errorf("unable to write %w", err)
	}
	var line string
	if err := conn.(*net.TCPConn).SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	_, err = fmt.Fscan(conn, &line)
	if err != nil {
		return nil, fmt.Errorf("unable to read %w", err)
	}
	if strings.HasPrefix(line, "info") {
		dev.Start(func() (io.ReadWriteCloser, error) {
			dev.SetMetadata("addr", addr)
			return dialerFn()
		})
		return dev, nil
	}
	return nil, errors.New("invalid protocol")
}

func Discover(network string) (devs []*Device) {
	ip := parseIp(network)
	if ip[3] == 0 {
		ip[3] = 1
	}
	lock := new(sync.Mutex)
	start := ip[3]
	wg := new(sync.WaitGroup)
	wg.Add(255 - start)
	for i := start; i < 255; i++ {
		addr := fmt.Sprintf("%d.%d.%d.%d:5000", ip[0], ip[1], ip[2], i)
		go func(addr string) {
			defer wg.Done()
			dev, err := Probe(addr)
			if err == nil {
				lock.Lock()
				devs = append(devs, dev)
				lock.Unlock()
			}
		}(addr)
	}
	wg.Wait()
	return
}
