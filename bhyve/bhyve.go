// Copyright 2019 Steve Wills. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bhyve

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
)

const (
	defaultDiskSize       = 16384 // Mb
	defaultMemSize        = 1024  // Mb
	defaultCPUCount       = 1
	defaultBridge         = "bridge0"
	defaultSubnet         = "192.168.8.1/24"
	defaultDHCPRange      = "192.168.8.10,192.168.8.254"
	defaultBoot2DockerURL = ""
	defaultISOFilename    = "boot2docker.iso"
	retrycount            = 16
	sleeptime             = 100 // milliseconds
	isoFilename           = "boot2docker.iso"
	diskname              = "guest.img"
)

type Driver struct {
	*drivers.BaseDriver
	EnginePort     int
	DiskSize       int64
	MemSize        int64
	CPUcount       int
	NetDev         string
	MACAddress     string
	Bridge         string
	DHCPRange      string
	NMDMDev        string
	Boot2DockerURL string
	Subnet         string
}

func (d *Driver) getBhyveVMName() (string, error) {

	username, err := user.Current()
	if err != nil {
		return "", err
	}

	return "docker-machine-" + username.Username + "-" + d.MachineName, nil
}

func (d *Driver) publicSSHKeyPath() string {
	return d.GetSSHKeyPath() + ".pub"
}

func (d *Driver) Create() error {
	if err := d.copyIsoToMachineDir(d.Boot2DockerURL, d.MachineName); err != nil {
		return err
	}

	if err := d.generateRawDiskImage(d.ResolveStorePath(diskname), d.DiskSize); err != nil {
		return err
	}

	log.Infof("Starting %s...", d.MachineName)
	if err := d.Start(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) DriverName() string {
	return "bhyve"
}

func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		mcnflag.IntFlag{
			EnvVar: "BHYVE_DISK_SIZE",
			Name:   "bhyve-disk-size",
			Usage:  "Size of disk for host in MB",
			Value:  defaultDiskSize,
		},
		mcnflag.IntFlag{
			EnvVar: "BHYVE_MEM_SIZE",
			Name:   "bhyve-mem-size",
			Usage:  "Size of memory for host in MB",
			Value:  defaultMemSize,
		},
		mcnflag.IntFlag{
			EnvVar: "BHYVE_CPUS",
			Name:   "bhyve-cpus",
			Usage:  "Number of CPUs in VM",
			Value:  defaultCPUCount,
		},
		mcnflag.StringFlag{
			Name:   "bhyve-bridge",
			Usage:  "Name of bridge interface",
			EnvVar: "BHYVE_BRIDGE",
			Value:  defaultBridge,
		},
		mcnflag.StringFlag{
			Name:   "bhyve-subnet",
			Usage:  "IP subnet to use",
			EnvVar: "BHYVE_SUBNET",
			Value:  defaultSubnet,
		},
		mcnflag.StringFlag{
			Name:   "bhyve-dhcprange",
			Usage:  "DHCP Range to use",
			EnvVar: "BHYVE_DHCPRANGE",
			Value:  defaultDHCPRange,
		},
		mcnflag.StringFlag{
			Name:   "bhyve-boot2docker-url",
			Usage:  "URL for boot2docker.iso",
			EnvVar: "BHYVE_BOOT2DOCKERURL",
		},
	}
}

func (d *Driver) GetIP() (string, error) {
	s, err := d.GetState()
	if err != nil {
		log.Debugf("Couldn't get state")
		return "", err
	}
	if s != state.Running {
		log.Debugf("Host not running")
		return "", drivers.ErrHostIsNotRunning
	}

	if d.IPAddress != "" {
		log.Debugf("Returning saved IP " + d.IPAddress)
		return d.IPAddress, nil
	}

	log.Debugf("getting IP from DHCP lease")
	ip, err := getIPfromDHCPLease(filepath.Join(d.StorePath, "bhyve.leases"), d.MACAddress)
	if err != nil {
		return "", err
	}
	d.IPAddress = ip
	return ip, nil
}

func (d *Driver) GetSSHHostname() (string, error) {
	return d.GetIP()
}

func (d *Driver) GetState() (state.State, error) {
	vmname, err := d.getBhyveVMName()
	if err != nil {
		return state.Stopped, nil
	}

	if fileExists("/dev/vmm/" + vmname) {
		log.Debugf("STATE: running")
		return state.Running, nil
	}
	return state.Stopped, nil
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tcp://%s:2376", ip), nil
}

func (d *Driver) Kill() error {
	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	if err := destroyVM(vmname); err != nil {
		return err
	}

	if err := destroyTap(d.NetDev); err != nil {
		return err
	}

	if err := killConsoleLogger(d.ResolveStorePath("nmdm.pid")); err != nil {
		return err
	}

	d.IPAddress = ""

	return nil
}

func (d *Driver) PreCreateCheck() error {

	err := ensureIPForwardingEnabled()
	if err != nil {
		return err
	}

	err = setupnet(d.Bridge, d.Subnet)
	if err != nil {
		return err
	}

	err = startDHCPServer(d.StorePath, d.Bridge, d.DHCPRange)
	if err != nil {
		return err
	}
	return nil
}

func (d *Driver) Remove() error {
	err := d.Kill()
	if err != nil {
		log.Debugf("Failed to kill %s, perhaps already dead?", d.MachineName)
	}

	err = os.RemoveAll(d.ResolveStorePath(diskname))
	if err != nil {
		return err
	}

	return nil
}

func (d *Driver) Restart() error {
	s, err := d.GetState()
	if err != nil {
		return err
	}
	if s == state.Running {
		if err := d.Stop(); err != nil {
			return err
		}
	}

	if err := d.Start(); err != nil {
		return err
	}

	return d.waitForIP()
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {

	d.DiskSize = int64(flags.Int("bhyve-disk-size")) * 1024 * 1024
	d.CPUcount = int(flags.Int("bhyve-cpus"))
	d.MemSize = int64(flags.Int("bhyve-mem-size"))
	d.MACAddress = generateMACAddress()
	d.SSHUser = "docker"
	d.Bridge = string(flags.String("bhyve-bridge"))
	d.Subnet = string(flags.String("bhyve-subnet"))
	d.DHCPRange = string(flags.String("bhyve-dhcprange"))
	d.Boot2DockerURL = flags.String("bhyve-boot2docker-url")

	return nil
}

func (d *Driver) Start() error {
	// TODO Need to fix this to log bhyve output to this file
	bhyvelogpath := d.ResolveStorePath("bhyve.log")
	log.Debugf("bhyvelogpath: %s", bhyvelogpath)

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	err = writeDeviceMap(d.ResolveStorePath("/device.map"), d.ResolveStorePath(isoFilename), d.ResolveStorePath(diskname))
	if err != nil {
		return err
	}

	err = runGrub(d.ResolveStorePath("/device.map"), strconv.Itoa(int(d.MemSize)), vmname)
	if err != nil {
		return err
	}

	nmdmdev, err := findNMDMDev()
	if err != nil {
		return err
	}
	d.NMDMDev = nmdmdev
	macaddr := d.MACAddress
	tapdev, err := findtapdev(d.Bridge)
	d.NetDev = tapdev
	cdpath := d.ResolveStorePath(isoFilename)
	cpucount := strconv.Itoa(int(d.CPUcount))
	ram := strconv.Itoa(int(d.MemSize))
	log.Debugf("RAM size: " + ram)

	err = startConsoleLogger(d.StorePath, nmdmdev)
	if err != nil {
		return err
	}

	cmd := exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s",
		"0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net,"+tapdev+",mac="+macaddr, "-s", "3:0,virtio-blk,"+
			d.ResolveStorePath(diskname), "-s", "4:0,virtio-rnd,/dev/random", "-s", "5:0,ahci-cd,"+cdpath, "-l", "com1,"+nmdmdev+"A", "-c", cpucount, "-m", ram+"M",
		vmname)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	slurp, _ := ioutil.ReadAll(stderr)
	log.Debugf("%s\n", slurp)

	if err := cmd.Wait(); err != nil {
		return err
	}
	log.Debugf("bhyve: " + stripCtlAndExtFromBytes(string(slurp)))

	if err := d.waitForIP(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Stop() error {
	err := d.Kill()
	if err != nil {
		return err
	}

	return nil
}

//noinspection GoUnusedExportedFunction
func NewDriver(hostName, storePath string) *Driver {
	return &Driver{
		EnginePort: engine.DefaultPort,
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
			StorePath:   storePath,
		},
		DiskSize:       defaultDiskSize,
		MemSize:        defaultMemSize,
		CPUcount:       defaultCPUCount,
		MACAddress:     "",
		Bridge:         defaultBridge,
		DHCPRange:      defaultDHCPRange,
		Boot2DockerURL: defaultBoot2DockerURL,
		Subnet:         defaultSubnet,
	}
}
