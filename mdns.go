package iotfwdrv

import (
	"context"
	"github.com/grandcat/zeroconf"
	"log"
	"strings"
)

func textLookup(key string, text []string) string {
	p := key + "="
	for _, t := range text {
		if strings.HasPrefix(t, p) {
			return strings.TrimPrefix(t, p)
		}
	}
	return ""
}

func HandleMDNS(ctx context.Context, onDiscover func(m MetadataAndAddr)) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalln("Failed to initialize resolver:", err.Error())
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for e := range entries {
			m := MetadataAndAddr{
				Metadata: Metadata{
					ID:    e.ServiceRecord.Instance,
					Name:  textLookup("name", e.Text),
					Model: textLookup("model", e.Text),
				},
			}
			m.HardwareVer, _ = ParseVersion(textLookup("hw", e.Text))
			m.FirmwareVer, _ = ParseVersion(textLookup("fw", e.Text))
			if len(e.AddrIPv4) > 0 {
				m.Addr.IP = e.AddrIPv4[0]
			}
			m.Addr.Port = e.Port
			onDiscover(m)
		}
	}()

	err = resolver.Browse(ctx, "_iotfw._tcp", "local.", entries)
	if err != nil {
		log.Fatalln("Failed to browse:", err.Error())
	}

	<-ctx.Done()
}
