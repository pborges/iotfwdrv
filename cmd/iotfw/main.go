package main

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/pborges/iotfwdrv"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)
import "github.com/spf13/cobra"

func main() {
	var rootCmd = &cobra.Command{
		Use:   "iotfw [sub]",
		Short: "Tools for interacting with the iotfw network",
	}

	var scanCmd = &cobra.Command{
		Use:   "scan",
		Short: "Scan for iotfw devices",
		Long:  "Scan for iotfw devices on the given networks",
		Run:   runScan,
	}

	var discoverCmd = &cobra.Command{
		Use:   "discover",
		Short: "Discover for iotfw devices",
		Long:  "Discover iotfw devices via mDNS",
		Run:   runDiscover,
	}

	var setCmd = &cobra.Command{
		Use:   "set [attr] [value]",
		Short: "Sets an attributes value on a remote device",
		Run:   runSet,
		Args:  cobra.ExactArgs(2),
	}
	setCmd.Flags().String("ip", "", "remote address in <ip> format")
	setCmd.MarkFlagRequired("ip")
	setCmd.Flags().Int("port", 5000, "port (5000 default)")

	var getCmd = &cobra.Command{
		Use:   "get [attr]",
		Short: "Gets an attribute value from a remote device",
		Run:   runGet,
		Args:  cobra.ExactArgs(1),
	}
	getCmd.Flags().String("ip", "", "remote address in <ip> format")
	getCmd.MarkFlagRequired("ip")
	getCmd.Flags().Int("port", 5000, "port (5000 default)")

	var subCmd = &cobra.Command{
		Use:   "sub [filter]",
		Short: "Subscribe to events from a remote device",
		Run:   runSub,
		Args:  cobra.ExactArgs(1),
	}
	subCmd.Flags().String("ip", "", "remote address in <ip> format")
	subCmd.MarkFlagRequired("ip")
	subCmd.Flags().Int("port", 5000, "port (5000 default)")

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(subCmd)
	rootCmd.AddCommand(discoverCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runGet(cmd *cobra.Command, args []string) {
	ip, err := cmd.Flags().GetString("ip")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	dev := iotfwdrv.New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", addr, 2*time.Second)
	})
	dev.Log.SetOutput(os.Stdout)

	if err := dev.Connect(); err == nil {
		fmt.Println("get", args[0], "value:", dev.Get(args[0]))
	} else {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runSet(cmd *cobra.Command, args []string) {
	ip, err := cmd.Flags().GetString("ip")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	dev := iotfwdrv.New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", addr, 2*time.Second)
	})
	dev.Log.SetOutput(os.Stdout)

	if err := dev.Connect(); err == nil {
		fmt.Println("set", args[0], args[1], "err:", dev.Set(args[0], args[1]))
	} else {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runSub(cmd *cobra.Command, args []string) {
	ip, err := cmd.Flags().GetString("ip")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	dev := iotfwdrv.New(func() (io.ReadWriteCloser, error) {
		return net.DialTimeout("tcp", addr, 2*time.Second)
	})
	dev.Log.SetOutput(os.Stdout)

	if err := dev.Connect(); err == nil {
		fmt.Println("sub", args[0])
		for m := range dev.Subscribe(args[0]).Chan() {
			fmt.Println(m.Key, m.Value)
		}
	} else {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runScan(cmd *cobra.Command, args []string) {
	networks, err := iotfwdrv.LocalNetworks()
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	if len(args) > 0 {
		networks = make([]net.IP, 0)
		for _, a := range args {
			networks = append(networks, net.ParseIP(a))
		}
	}

	fmt.Println("Attempting discovery on", networks)
	devs, devErrs := iotfwdrv.Scan(networks...)

	renderMetadataTable(devs...)

	if showErrors, err := cmd.Flags().GetBool("errors"); err == nil && showErrors {
		if ipErrs, ok := devErrs.(iotfwdrv.IPErrors); ok {
			// dump errors
			sort.Slice(ipErrs, func(i, j int) bool {
				return ipErrs[i].IP.To4()[3] < ipErrs[j].IP.To4()[3]
			})
			for _, e := range ipErrs {
				fmt.Println(e.Error())
			}
		}
	}
}

func runDiscover(cmd *cobra.Command, args []string) {
	devCh := make(chan iotfwdrv.MetadataAndAddr)

	go func() {
		devMap := make(map[string]iotfwdrv.MetadataAndAddr)
		var devs []iotfwdrv.MetadataAndAddr
		for m := range devCh {
			if _, ok := devMap[m.ID]; !ok {
				devs = append(devs, m)
			}
			devMap[m.ID] = m
			renderMetadataTable(devs...)
		}
	}()

	fmt.Println(iotfwdrv.HandleMDNS(func(m iotfwdrv.MetadataAndAddr) {
		devCh <- m
	}))
}

func renderMetadataTable(devs ...iotfwdrv.MetadataAndAddr) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Model", "HW VER", "FW VER", "IP", "PORT"})
	table.SetFooter([]string{"", "", "", "", "", "TOTAL", strconv.Itoa(len(devs))})
	sort.Slice(devs, func(i, j int) bool {
		return strings.Compare(devs[i].Name, devs[j].Name) < 0
	})

	for _, dev := range devs {
		table.Append([]string{
			dev.ID,
			dev.Name,
			dev.Model,
			dev.HardwareVer.String(),
			dev.FirmwareVer.String(),
			dev.Addr.IP.String(),
			strconv.Itoa(dev.Addr.Port),
		})
	}

	table.Render()
}
