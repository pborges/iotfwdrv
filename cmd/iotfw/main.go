package main

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/pborges/iotfwdrv"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)
import "github.com/spf13/cobra"

func main() {
	var rootCmd = &cobra.Command{
		Use:   "iotfw [sub]",
		Short: "Tools for interacting with the iotfw network",
	}

	var discoverCmd = &cobra.Command{
		Use:   "discover [network] [network] [network]...",
		Short: "Discover iotfw devices",
		Long:  "Discover iotfw devices on given networks, if none are supplied, iotfw will attempt to use all networks this device is a part of",
		Run:   runDiscover,
	}
	discoverCmd.Flags().Bool("errors", false, "display all errors")

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

	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(getCmd)

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

func runDiscover(cmd *cobra.Command, args []string) {
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
	devs, errs := iotfwdrv.Discover(networks...)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Model", "HW VER", "FW VER", "Addr"})

	sort.Slice(devs, func(i, j int) bool {
		return strings.Compare(devs[i].Info().Name, devs[j].Info().Name) < 0
	})

	for _, dev := range devs {
		table.Append([]string{dev.Info().ID, dev.Info().Name, dev.Info().Model, dev.Info().HardwareVer.String(), dev.Info().FirmwareVer.String(), dev.Addr().String()})
	}

	table.Render()

	if showErrors, err := cmd.Flags().GetBool("errors"); err == nil && showErrors {
		// dump errors
		sort.Slice(errs, func(i, j int) bool {
			return errs[i].(iotfwdrv.IPError).IP.To4()[3] < errs[j].(iotfwdrv.IPError).IP.To4()[3]
		})
		for _, err := range errs {
			fmt.Println(err.Error())
		}
	}
}
