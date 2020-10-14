package iotfwdrv

import (
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

func probe(addr string) (dev *Device, err error) {
	dev = New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", addr, 2*time.Second)
	})
	err = dev.Connect()
	return
}

func Discover(networks ...net.IP) (devs []*Device) {
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	wg.Add(254 * len(networks))

	for _, network := range networks {
		for i := 1; i < 255; i++ {
			ip := network
			ip[3] = byte(i)
			addr := fmt.Sprintf("%s:5000", ip)
			go func(addr string) {
				defer wg.Done()
				dev, err := probe(addr)
				if err == nil {
					lock.Lock()
					devs = append(devs, dev)
					lock.Unlock()
				}
			}(addr)
		}
	}

	wg.Wait()
	sort.Slice(devs, func(i, j int) bool {
		return strings.Compare(devs[i].Info().ID, devs[j].Info().ID) < 0
	})
	return
}

func LocalNetworks() ([]net.IP, error) {
	networks := make([]net.IP, 0)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				networks = append(networks, ip.Mask(ip.DefaultMask()))
			}
		}
	}
	return networks, nil
}
