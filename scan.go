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

type IPError struct {
	net.IP
	error
}

type IPErrors []IPError

func (e IPErrors) Error() string {
	return fmt.Sprintf("%d endpoints returned an error", len(e))
}

func Scan(networks ...net.IP) (devs []MetadataAndAddr, err error) {
	type res struct {
		IP net.IP
		*Device
		error
	}

	addrCount := 254 * len(networks)

	in := make(chan net.IP, addrCount)
	out := make(chan res)

	// load teh queue
	for _, network := range networks {
		network = network.To4()
		for i := 1; i < 255; i++ {
			ip := net.IPv4(network[0], network[1], network[2], byte(i))
			in <- ip
		}
	}

	for i := 0; i < 64; i++ {
		go func() {
			for {
				select {
				case ip := <-in:
					addr := fmt.Sprintf("%s:%d", ip.To4(), 5000)
					dev, err := probe(addr)
					out <- res{IP: ip, Device: dev, error: fmt.Errorf("[%s] %w", addr, err)}
				default:
					return
				}
			}
		}()
	}
	var devErr IPErrors
	for i := 0; i < addrCount; i++ {
		res := <-out
		if res.Device != nil && res.Device.Connected() {
			devs = append(devs, MetadataAndAddr{
				Metadata: res.Device.Info(),
				Addr:     *res.Device.Addr(),
			})
			_ = res.Device.Disconnect()
		} else if res.error != nil {
			devErr = append(devErr, IPError{IP: res.IP, error: res.error})
		}
	}
	err = devErr

	sort.Slice(devs, func(i, j int) bool {
		return strings.Compare(devs[i].ID, devs[j].ID) < 0
	})

	return
}

func LocalNetworks() ([]net.IP, error) {
	networksMap := make(map[string]net.IP)
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
				networksMap[a.IP.Mask(a.Mask).String()] = a.IP.Mask(a.Mask)
			}
		}
	}

	networks := make([]net.IP, 0, len(networksMap))
	for _, n := range networksMap {
		networks = append(networks, n)
	}

	return networks, nil
}
