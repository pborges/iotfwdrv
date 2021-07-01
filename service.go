package iotfwdrv

import (
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

const maxDatagramSize = 256

type DeviceContext struct {
	*Device
	ConnectedAt time.Time
	reconnect   bool
}

type ServicePlugin interface {
	ServiceName() string
}

type ServicePluginOnRegister interface {
	OnRegister(ctx DeviceContext)
}

type ServicePluginOnUnregister interface {
	OnUnregister(ctx DeviceContext)
}

type ServicePluginOnConnect interface {
	OnConnect(ctx DeviceContext)
}

type ServicePluginOnDisconnect interface {
	OnDisconnect(ctx DeviceContext)
}

type Service struct {
	Networks      []net.IP
	Log           *log.Logger
	OnRegister    func(ctx DeviceContext)
	OnUnregister  func(ctx DeviceContext)
	OnConnect     func(ctx DeviceContext)
	OnDisconnect  func(ctx DeviceContext)
	Plugins       []ServicePlugin
	devices       map[string]*DeviceContext
	fnCh          chan func()
	subscriptions []*Subscription
}

func (s *Service) multicastAddr() *net.UDPAddr {
	multicastAddr, err := net.ResolveUDPAddr("udp", "226.1.13.37:5000")
	if err != nil {
		panic(err)
	}
	return multicastAddr
}

func (s *Service) exec(fn func()) {
	if s.fnCh == nil {
		s.fnCh = make(chan func())
		go func() {
			for fn := range s.fnCh {
				fn()
			}
		}()
		s.exec(s.init)
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.fnCh <- func() {
		fn()
		wg.Done()
	}
	wg.Wait()
}

func (s *Service) init() {
	if s.devices == nil {
		s.devices = make(map[string]*DeviceContext)
	}
	s.setupMulticastDiscovery()
}

func (s *Service) logf(format string, a ...interface{}) {
	if s.Log != nil {
		s.Log.Printf(format, a...)
	}
}

func (s *Service) setupMulticastDiscovery() {
	s.logf("setup multicast discovery")
	go func() {
		conn, err := net.ListenMulticastUDP("udp", nil, s.multicastAddr())
		if err != nil {
			log.Fatal(err)
		}

		conn.SetReadBuffer(maxDatagramSize)
		for {
			b := make([]byte, maxDatagramSize)
			n, src, err := conn.ReadFromUDP(b)
			if err != nil {
				s.logf("ReadFromUDP failed: %s", err.Error())
				continue
			}
			res := string(b[:n])
			if strings.HasPrefix(res, "iotfw found ") {
				id := strings.TrimPrefix(res, "iotfw found ")

				if _, ok := s.devices[id]; !ok {
					s.logf("found via multicast %s", res)
					dev := New(func() (io.ReadWriteCloser, error) {
						return net.DialTimeout("tcp", src.String(), 4*time.Second)
					})
					if err := dev.Connect(); err == nil {
						s.discovered(dev)
					} else {
						s.logf("unable to connect to discovered device %s", err.Error())
					}
				}
			}
		}
	}()
}

func (s *Service) fanout(info Info, key string, value string) {
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

func (s *Service) discovered(dev *Device) {
	// did the IP change or something?
	if ctx, ok := s.devices[dev.Info().ID]; ok {
		if !ctx.Connected() || dev.Addr().String() != ctx.Addr().String() {
			s.logf("[%s:%s] unregistering device connected: %t existingIP: %s newIp: %s",
				ctx.Info().ID,
				ctx.Info().Name,
				ctx.Connected(),
				ctx.Addr().String(),
				dev.Addr().String(),
			)
			ctx.reconnect = false
			ctx.Disconnect()
			delete(s.devices, dev.Info().ID)

			if s.OnUnregister != nil {
				s.OnUnregister(*ctx)
			}
			for _, p := range s.Plugins {
				if fn, ok := p.(ServicePluginOnUnregister); ok {
					s.logf("executing %s->OnUnregister for %s (%s)", p.ServiceName(), dev.Info().ID, dev.info.Name)
					fn.OnUnregister(*ctx)
				}
			}
		}
	}

	if _, ok := s.devices[dev.Info().ID]; !ok {
		s.logf("[%s:%s] discovered device", dev.Info().ID, dev.Info().Name)
		ctx := &DeviceContext{
			Device:    dev,
			reconnect: true,
		}
		s.logf("[%s:%s] registering device", ctx.Info().ID, ctx.Info().Name)
		s.devices[dev.Info().ID] = ctx
		if s.OnRegister != nil {
			s.OnRegister(*ctx)
		}
		for _, p := range s.Plugins {
			if fn, ok := p.(ServicePluginOnRegister); ok {
				s.logf("executing %s->OnRegister for %s (%s)", p.ServiceName(), dev.Info().ID, dev.info.Name)
				fn.OnRegister(*ctx)
			}
		}
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
}

func (s *Service) Discover() (err error) {
	s.exec(func() {
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
		discovered, _ := Discover(s.Networks...)

		for _, dev := range discovered {
			s.discovered(dev)
		}
	})
	return
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
