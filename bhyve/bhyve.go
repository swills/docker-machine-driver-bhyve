package bhyve

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
)

const (
	defaultTimeout  = 15 * time.Second
	defaultDiskSize = 16384 // Mb
	defaultMemSize  = 1024  // Mb
	defaultCPUCount = 1
	defaultSSHPort  = 22
	retrycount      = 16
	sleeptime       = 100 // milliseconds
)

type Driver struct {
	*drivers.BaseDriver
	EnginePort int
	DiskSize   int64
	MemSize    int64
	CPUcount   int
	NetDev     string
}

// https://rosettacode.org/wiki/Strip_control_codes_and_extended_characters_from_a_string#Go
func stripCtlAndExtFromBytes(str string) string {
	b := make([]byte, len(str))
	var bl int
	for i := 0; i < len(str); i++ {
		c := str[i]
		if c >= 32 && c < 127 {
			b[bl] = c
			bl++
		}
	}
	return string(b[:bl])
}

func easyCmd(args ...string) error {
	log.Debugf("EXEC: " + strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	log.Debugf("STDOUT: %s", stdout.String())
	log.Debugf("STDERR: %s", stderr.String())
	return err
}

func findnmdmdev() (string, error) {
	return "/dev/nmdm1A", nil
}

func findmacaddr() (string, error) {
	return "00:A0:98:00:00:02", nil
}

func findtapdev() (string, error) {
	lasttap := 0
	numtaps := 0
	nexttap := 0
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		match, _ := regexp.MatchString("^tap", iface.Name)
		if match {
			r := regexp.MustCompile(`tap(?P<num>\d*)`)
			res := r.FindAllStringSubmatch(iface.Name, -1)
			tapnum, err := strconv.Atoi(res[0][1])
			if err != nil {
			}
			if tapnum > lasttap {
				lasttap = tapnum
			}
			numtaps = numtaps + 1
		}
	}
	if numtaps > 0 {
		nexttap = lasttap + 1
	}
	log.Debugf("nexttap: %d", nexttap)

	nexttapname := "tap" + strconv.Itoa(nexttap)
	err := easyCmd("sudo", "ifconfig", nexttapname, "create")
	if err != nil {
		return "", err
	}

	err = easyCmd("sudo", "ifconfig", "bridge0", "addm", nexttapname)
	if err != nil {
		return "", err
	}

	err = easyCmd("sudo", "ifconfig", nexttapname, "up")
	if err != nil {
		return "", err
	}

	return nexttapname, nil
}

func findcdpath() (string, error) {
	return "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/boot2docker.iso", nil
}

func (d *Driver) getBhyveVMName() (string, error) {
	username, err := getUsername()
	if err != nil {
		return "", err
	}

	return "docker-machine-" + username + "-" + d.MachineName, nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (d *Driver) writeDeviceMap() error {
	devmap := d.ResolveStorePath("/device.map")

	f, err := os.Create(devmap)
	if err != nil {
		return err
	}

	_, err = f.WriteString("(hd0) " + d.ResolveStorePath("guest.img") + "\n")
	if err != nil {
		return err
	}
	_, err = f.WriteString("(cd0) /usr/home/swills/Documents/git/docker-machine-driver-bhyve/boot2docker.iso\n")
	if err != nil {
		return err
	}

	err = f.Sync()
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	return nil

}

func (d *Driver) runGrub() error {
	for maxtries := 0; maxtries < retrycount; maxtries++ {
		err := d.writeDeviceMap()
		if err != nil {
			return err
		}

		vmname, err := d.getBhyveVMName()
		if err != nil {
			return err
		}

		cmd := exec.Command("sudo", "/usr/local/sbin/grub-bhyve", "-m", d.ResolveStorePath("device.map"), "-r", "cd0", "-M",
			strconv.Itoa(int(d.MemSize))+"M", vmname)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		go func() {
			_, err = io.WriteString(stdin, "linux (cd0)/boot/vmlinuz waitusb=5:LABEL=boot2docker-data base norestore noembed\n")
			if err != nil {
				return
			}
			_, err = io.WriteString(stdin, "initrd (cd0)/boot/initrd.img\n")
			if err != nil {
				return
			}
			_, err = io.WriteString(stdin, "boot\n")
			if err != nil {
				return
			}
			err = stdin.Close()
			if err != nil {
				return
			}
		}()

		out, err := cmd.CombinedOutput()
		log.Debugf("grub-bhyve: " + stripCtlAndExtFromBytes(string(out)))
		if strings.Contains(string(out), "GNU GRUB") {
			log.Debugf("grub-bhyve: looks OK")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func (d *Driver) CreateDiskImage(vmpath string) error {
	err := easyCmd("truncate", "-s", strconv.Itoa(int(d.DiskSize)), vmpath)
	if err != nil {
		return err
	}

	err = easyCmd("dd", "if=/usr/home/swills/Documents/git/docker-machine-driver-bhyve/userdata.tar",
		"of="+vmpath, "conv=notrunc", "status=none")
	if err != nil {
		return err
	}

	return nil
}

func getUsername() (string, error) {
	username, err := user.Current()
	if err != nil {
		return "", err
	}

	return username.Username, nil
}

func (d *Driver) Create() error {
	log.Debugf("Create called")

	vmpath := d.ResolveStorePath("guest.img")
	bhyvelogpath := d.ResolveStorePath("bhyve.log")
	log.Debugf("vmpath: %s", vmpath)
	log.Debugf("bhyvelogpath: %s", bhyvelogpath)
	log.Debugf("Deleting %s", vmpath)

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	err = d.CreateDiskImage(vmpath)
	if err != nil {
		return err
	}

	err = d.runGrub()
	if err != nil {
		return err
	}

	nmdmdev, err := findnmdmdev()
	macaddr, err := findmacaddr()
	tapdev, err := findtapdev()
	d.NetDev = tapdev
	cdpath, err := findcdpath()
	cpucount := strconv.Itoa(int(d.CPUcount))
	ram := strconv.Itoa(int(d.MemSize))
	log.Debugf("RAM size: " + ram)

	// cmd = exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-o", bhyvelogpath, "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
	cmd := exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s",
		"0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net,"+tapdev+",mac="+macaddr, "-s", "3:0,virtio-blk,"+
			vmpath, "-s", "4:0,ahci-cd,"+cdpath, "-l", "com1,"+nmdmdev, "-c", cpucount, "-m", ram+"M",
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

	/*
		err = easyCmd("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
		if err != nil {
			return err
		}
	*/

	err = easyCmd("cp", "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/id_rsa",
		d.ResolveStorePath("/id_rsa"))
	if err != nil {
		return err
	}
	return nil
}

func (d *Driver) DriverName() string {
	log.Debugf("DriverName called")
	return "bhyve"
}

func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	log.Debugf("GetCreateFlags called")
	return []mcnflag.Flag{
		mcnflag.IntFlag{
			EnvVar: "BHYVE_DISK_SIZE",
			Name:   "bhyve-ssh-port",
			Usage:  "Port to use for SSH",
			Value:  defaultSSHPort,
		},
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
		mcnflag.IntFlag{
			Name:   "bhyve-engine-port",
			Usage:  "Docker engine port",
			Value:  engine.DefaultPort,
			EnvVar: "BHYVE_ENGINE_PORT",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-ip-address",
			Usage:  "IP Address of machine",
			EnvVar: "BHYVE_IP_ADDRESS",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-ssh-user",
			Usage:  "SSH user",
			Value:  drivers.DefaultSSHUser,
			EnvVar: "BHYVE_SSH_USER",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-ssh-key",
			Usage:  "SSH private key path (if not provided, default SSH key will be used)",
			Value:  "",
			EnvVar: "BHYVE_SSH_KEY",
		},
	}
}

func (d *Driver) GetIP() (string, error) {
	log.Debugf("GetIP called")
	return d.IPAddress, nil
}

func (d *Driver) GetSSHHostname() (string, error) {
	log.Debugf("GetSSHHostname called")
	return d.GetIP()
}

func (d *Driver) GetState() (state.State, error) {
	log.Debugf("GetState called")

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return state.Stopped, nil
	}

	if !fileExists("/dev/vmm/" + vmname) {

		return state.Stopped, nil
	}

	address := net.JoinHostPort(d.IPAddress, strconv.Itoa(d.SSHPort))

	_, err = net.DialTimeout("tcp", address, defaultTimeout)
	if err != nil {
		log.Debugf("STATE: stopped")
		return state.Stopped, nil
	}
	log.Debugf("STATE: running")
	return state.Running, nil
}

func (d *Driver) GetURL() (string, error) {
	log.Debugf("GetURL called")
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tcp://%s:2376", ip), nil
}

func (d *Driver) Kill() error {
	log.Debugf("Kill called")

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	tries := 0
	for ; tries < retrycount; tries++ {
		if fileExists("/dev/vmm/" + vmname) {
			log.Debugf("Removing VM %s, try %d", d.MachineName, tries)
			err := easyCmd("sudo", "bhyvectl", "--destroy", "--vm="+vmname)
			if err != nil {
			}
			time.Sleep(sleeptime * time.Millisecond)
		}
	}

	if tries > retrycount {
		return fmt.Errorf("failed to kill %d", d.MachineName)
	}

	err = easyCmd("sudo", "ifconfig", d.NetDev, "destroy")
	if err != nil {
		return err
	}

	return nil
}

func (d *Driver) PreCreateCheck() error {
	log.Debugf("preCreateCheck called")
	return nil
}

func (d *Driver) Remove() error {
	log.Debugf("Remove called")

	vmpath := d.ResolveStorePath("guest.img")

	err := os.RemoveAll(vmpath)
	if err != nil {
		return err
	}

	return nil
}

func (d *Driver) Restart() error {
	log.Debugf("Restart called")
	return errors.New("not implemented yet")
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	log.Debugf("SetConfigFromFlags called")

	disksize := int64(flags.Int("bhyve-disk-size")) * 1024 * 1024
	log.Debugf("Setting disk size to %d", disksize)
	d.DiskSize = disksize

	cpucount := int(flags.Int("bhyve-cpus"))
	log.Debugf("Setting CPU count to %d", cpucount)
	d.CPUcount = cpucount

	memsize := int64(flags.Int("bhyve-mem-size"))
	log.Debugf("Setting mem size to %d", memsize)
	d.MemSize = memsize

	log.Debugf("Setting ip address to", flags.String("bhyve-ip-address"))
	d.IPAddress = flags.String("bhyve-ip-address")

	d.SSHUser = "docker"
	log.Debugf("Setting port to", flags.String("bhyve-ssh-port"))
	d.SSHPort = flags.Int("bhyve-ssh-port")

	return nil
}

func (d *Driver) Start() error {
	log.Debugf("Start called")
	return errors.New("not implemented yet")
}

func (d *Driver) Stop() error {
	log.Debugf("Stop called")
	return errors.New("not implemented yet")
}

//noinspection GoUnusedExportedFunction
func NewDriver(hostName, storePath string) *Driver {
	log.Debugf("NewDriver called")
	return &Driver{
		EnginePort: engine.DefaultPort,
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
			StorePath:   storePath,
		},
		DiskSize: defaultDiskSize,
		MemSize:  defaultMemSize,
		CPUcount: defaultCPUCount,
	}
}
