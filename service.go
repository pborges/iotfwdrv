package iotfwdrv

import (
	"context"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

const MdnsService = "_iotfw._tcp.local."

type Metadata struct {
	ID          string
	Name        string
	Model       string
	HardwareVer Version
	FirmwareVer Version
}

type MetadataAndAddr struct {
	Metadata
	Addr net.TCPAddr
}

type DeviceContext struct {
	*Device
	ConnectedAt time.Time
	reconnect   bool
}

type ServicePlugin interface {
	ServiceName() string
}

type ServicePluginOnRegister interface {
	OnRegister(m MetadataAndAddr)
}

type ServicePluginOnConnect interface {
	OnConnect(ctx DeviceContext)
}

type ServicePluginOnDisconnect interface {
	OnDisconnect(ctx DeviceContext)
}

type Service struct {
	Networks       []net.IP
	Log            *log.Logger
	OnRegister     func(m MetadataAndAddr)
	OnConnect      func(ctx DeviceContext)
	OnDisconnect   func(ctx DeviceContext)
	Plugins        []ServicePlugin
	devices        map[string]*DeviceContext
	fnCh           chan func()
	subscriptions  []*Subscription
	mdnsCancelFunc context.CancelFunc
}

func (s *Service) exec(fn func()) {
	if s.fnCh == nil {
		s.fnCh = make(chan func())
		s.devices = make(map[string]*DeviceContext)
		go func() {
			for fn := range s.fnCh {
				fn()
			}
		}()
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.fnCh <- func() {
		fn()
		wg.Done()
	}
	wg.Wait()
}

func (s *Service) HandleMDNS() {
	s.logf("setup mdns discovery")

	HandleMDNS(s.Register)
}

func (s *Service) logf(format string, a ...interface{}) {
	if s.Log != nil {
		s.Log.Output(2, fmt.Sprintf(format, a...))
	}
}

func (s *Service) fanout(info Metadata, key string, value string) {
	slowSubscribers := make([]*Subscription, 0, len(s.subscriptions))
	key = fmt.Sprintf("%s.%s", info.ID, key)
	for _, sub := range s.subscriptions {
		if KeyMatch(key, sub.filter) {
			select {
			case sub.ch <- Message{
				Device: info,
				Key:    key,
				Value:  value,
			}:
			default:
				slowSubscribers = append(slowSubscribers, sub)
			}
		}
	}
	for _, sub := range slowSubscribers {
		sub.Close()
		s.logf("closing slow subscriber %s", sub)
	}
}

func (s *Service) Device(id string) (dev *Device) {
	s.exec(func() {
		if ctx, ok := s.devices[id]; ok {
			dev = ctx.Device
		}
	})
	return
}

func (s *Service) Devices() []*Device {
	devs := make([]*Device, 0, len(s.devices))
	s.exec(func() {
		for _, dev := range s.devices {
			devs = append(devs, dev.Device)
		}
	})
	return devs
}

func (s *Service) Register(m MetadataAndAddr) {
	s.exec(func() {
		s.logf("[%s:%s] register device", m.ID, m.Name)
		if s.OnRegister != nil {
			s.OnRegister(m)
		}
		for _, p := range s.Plugins {
			if fn, ok := p.(ServicePluginOnRegister); ok {
				s.logf("executing %s->OnRegister for %s (%s)", p.ServiceName(), m.ID, m.Name)
				fn.OnRegister(m)
			}
		}

		// did we create a Device for this yet? Did the IP change or something?
		if ctx, ok := s.devices[m.ID]; ok {
			if !ctx.Connected() || m.Addr.String() != ctx.Addr().String() {
				s.logf("[%s:%s] unregistering device connected: %t existingIP: %s newIp: %s",
					ctx.Info().ID,
					ctx.Info().Name,
					ctx.Connected(),
					ctx.Addr().String(),
					m.Addr.String(),
				)
				ctx.reconnect = false
				ctx.Disconnect()
				delete(s.devices, m.ID)
			}
		}

		if _, ok := s.devices[m.ID]; !ok {
			s.logf("[%s:%s] registering device", m.ID, m.Name)

			// attempt to dial
			dev := New(func() (io.ReadWriteCloser, error) {
				return net.DialTimeout("tcp", m.Addr.String(), 4*time.Second)
			})

			if err := dev.Connect(); err != nil {
				s.logf("unable to connect to Register device %s", err.Error())
				return
			}

			ctx := &DeviceContext{
				Device:    dev,
				reconnect: true,
			}
			s.logf("[%s:%s] registering device", ctx.Info().ID, ctx.Info().Name)
			s.devices[dev.Info().ID] = ctx

			go func(ctx *DeviceContext) {
				for ctx.reconnect {
					connectErr := ctx.Connect()
					if connectErr == nil {
						s.logf("[%s:%s] connected", ctx.Info().ID, ctx.Info().Name)
						ctx.ConnectedAt = time.Now()
						s.fanout(ctx.Device.Info(), KeyEvent, KeyEventConnect)

						// subscribe to everything and fanout
						go func(ctx *DeviceContext) {
							for m := range ctx.Subscribe(">").Chan() {
								s.fanout(ctx.Device.Info(), m.Key, m.Value)
							}
						}(ctx)

						if s.OnConnect != nil {
							s.OnConnect(*ctx)
						}
						for _, p := range s.Plugins {
							if fn, ok := p.(ServicePluginOnConnect); ok {
								s.logf("executing %s->OnConnect for %s (%s)", p.ServiceName(), dev.Info().ID, dev.info.Name)
								fn.OnConnect(*ctx)
							}
						}
						waitErr := ctx.Wait()
						s.logf("[%s:%s] disconnect uptime: %s err: %+v", ctx.Info().ID, ctx.Info().Name, time.Since(ctx.ConnectedAt), waitErr)
						s.fanout(ctx.Device.Info(), KeyEvent, KeyEventDisconnect)

						if s.OnDisconnect != nil {
							s.OnDisconnect(*ctx)
						}
						for _, p := range s.Plugins {
							if fn, ok := p.(ServicePluginOnDisconnect); ok {
								s.logf("executing %s->OnDisconnect for %s (%s)", p.ServiceName(), dev.Info().ID, dev.info.Name)
								fn.OnDisconnect(*ctx)
							}
						}
					} else {
						s.logf("[%s:%s] connect err: %+v", ctx.Info().ID, ctx.Info().Name, connectErr)
						time.Sleep(5 * time.Second)
					}
				}
				s.logf("[%s:%s] disabling reconnect err:%+v", ctx.Info().ID, ctx.Info().Name, ctx.Wait())
			}(ctx)
		}
	})
}

func (s *Service) Subscribe(filter string) *Subscription {
	sub := &Subscription{
		filter: filter,
		ch:     make(chan Message, 10),
		execCh: s.fnCh,
	}
	s.exec(func() {
		s.subscriptions = append(s.subscriptions, sub)
	})
	return sub
}

func (s Service) RenderDevicesTable(w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"ID", "Name", "Model", "HW VER", "FW VER", "Addr", "Uptime"})

	s.exec(func() {
		keys := make([]string, len(s.devices))
		for key := range s.devices {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return strings.Compare(keys[i], keys[j]) < 0
		})

		for _, key := range keys {
			if ctx, ok := s.devices[key]; ok {
				uptime := "Not Connected"

				if ctx.Connected() {
					uptime = time.Since(ctx.ConnectedAt).String()
				}
				table.Append([]string{ctx.Info().ID, ctx.Info().Name, ctx.Info().Model, ctx.Info().HardwareVer.String(), ctx.Info().FirmwareVer.String(), ctx.Addr().String(), uptime})
			}
		}
	})
	table.Render()
}

func (s *Service) ScanAndRegister() (err error) {
	if s.Networks == nil {
		s.Networks, err = LocalNetworks()
		if err != nil {
			return
		}
	}

	strNet := make([]string, 0, len(s.Networks))
	for _, n := range s.Networks {
		strNet = append(strNet, n.To4().String())
	}
	s.logf("Attempting discovery on %s", strings.Join(strNet, ", "))
	var devs []MetadataAndAddr
	devs, err = Scan(s.Networks...)
	for _, m := range devs {
		s.Register(m)
	}
	return
}
