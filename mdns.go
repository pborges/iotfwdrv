package iotfwdrv

import (
	"context"
	"fmt"
	"github.com/brutella/dnssd"
)

func HandleMDNS(onDiscover func(m MetadataAndAddr)) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addFn := func(e dnssd.BrowseEntry) {
		go func(e dnssd.BrowseEntry) {
			m := MetadataAndAddr{
				Metadata: Metadata{
					ID:    e.Name,
					Name:  e.Text["name"],
					Model: e.Text["model"],
				},
			}
			m.HardwareVer, _ = ParseVersion(e.Text["hw"])
			m.FirmwareVer, _ = ParseVersion(e.Text["ver"])
			m.Addr.IP = e.IPs[0]

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if svc, err := dnssd.LookupInstance(ctx, e.ServiceInstanceName()); err == nil {
				if len(e.IPs) > 0 {
					m.Addr.Port = svc.Port
				} else {
					fmt.Println("no svc ips")
				}
			} else {
				fmt.Printf("unable to lookup mdns service for %s err: %s\n", e.ServiceInstanceName(), err)
			}

			onDiscover(m)
		}(e)
	}

	rmvFn := func(e dnssd.BrowseEntry) {}

	return dnssd.LookupType(ctx, MdnsService, addFn, rmvFn)
}
