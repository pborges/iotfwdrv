package iotfwdrv

import (
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"time"
)

func probe(addr string) (dev *Device, err error) {
	dev = New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", addr, 2*time.Second)
	})
	err = dev.Connect()
	return
}

func Discover(networks ...net.IP) (devs []*Device, errs []error) {
	type res struct {
		*Device
		error
	}

	addrCount := 254 * len(networks)

	in := make(chan string, addrCount)
	out := make(chan res)

	// load teh queue
	for _, network := range networks {
		network = network.To4()
		for i := 1; i < 255; i++ {
			ip := net.IPv4(network[0], network[1], network[2], byte(i))
			in <- fmt.Sprintf("%s:5000", ip)
		}
	}

	for i := 0; i < 64; i++ {
		go func() {
			for {
				select {
				case addr := <-in:
					dev, err := probe(addr)
					out <- res{Device: dev, error: fmt.Errorf("[%s] %w", addr, err)}
				default:
					return
				}
			}
		}()
	}

	for i := 0; i < addrCount; i++ {
		res := <-out
		if res.Device != nil && res.Device.Connected() {
			devs = append(devs, res.Device)
			_ = res.Device.Disconnect()
		} else if res.error != nil {
			errs = append(errs, res.error)
		}
	}

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
			if a, ok := addr.(*net.IPNet); ok &&
				!a.IP.IsLoopback() &&
				a.IP.To4() != nil {
				networks = append(networks, a.IP.Mask(a.Mask))
			}
		}
	}
	return networks, nil
}
