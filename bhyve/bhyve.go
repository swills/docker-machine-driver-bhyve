package bhyve

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
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
	defaultTimeout  = 5 * time.Second
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
	MACAddress string
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

// https://sosedoff.com/2014/12/15/generate-random-hex-string-in-go.html

func randomHex(n int) (string, error) {
	randbytes := make([]byte, n)
	if _, err := rand.Read(randbytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randbytes), nil
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
	lastnmdm := 0

	for {
		nmdmdev := "/dev/nmdm" + strconv.Itoa(lastnmdm) + "A"
		log.Debugf("checking nmdm: %s", nmdmdev)
		cmd := exec.Command("sudo", "fuser", nmdmdev)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return "", err
		}
		out := stdout.String()
		// Check if fuser reported anything
		log.Debugf("status: %s", out)
		words := strings.Fields(out)
		if len(words) < 1 {
			log.Debugf("using %s", nmdmdev)
			return nmdmdev, nil
		} else {
			log.Debugf("can't use %s, trying next device", nmdmdev)
			lastnmdm++
		}
		if lastnmdm > 100 {
			return "", errors.New("Could not find nmdm dev")
		}
		time.Sleep(1 * time.Second)
	}
}

func generatemacaddr() (string, error) {
	oidprefix := "58:9c:fc"
	b1, err := randomHex(1)
	if err != nil {
		return "", err
	}
	b2, err := randomHex(1)
	if err != nil {
		return "", err
	}
	b3, err := randomHex(1)
	if err != nil {
		return "", err
	}
	return oidprefix + ":" + b1 + ":" + b2 + ":" + b3, nil
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
	err := d.writeDeviceMap()
	if err != nil {
		return err
	}

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	for maxtries := 0; maxtries < retrycount; maxtries++ {
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
		time.Sleep(sleeptime * time.Millisecond)
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
	log.Debugf("vmpath: %s", vmpath)
	bhyvelogpath := d.ResolveStorePath("bhyve.log")
	log.Debugf("bhyvelogpath: %s", bhyvelogpath)

	err := d.CreateDiskImage(vmpath)
	if err != nil {
		return err
	}

	log.Infof("Starting %s...", d.MachineName)
	if err := d.Start(); err != nil {
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
			Usage:  "Port to use for SSH (default: 22)",
			Value:  defaultSSHPort,
		},
		mcnflag.IntFlag{
			EnvVar: "BHYVE_DISK_SIZE",
			Name:   "bhyve-disk-size",
			Usage:  "Size of disk for host in MB (default: 16384)",
			Value:  defaultDiskSize,
		},
		mcnflag.IntFlag{
			EnvVar: "BHYVE_MEM_SIZE",
			Name:   "bhyve-mem-size",
			Usage:  "Size of memory for host in MB (default: 1024)",
			Value:  defaultMemSize,
		},
		mcnflag.IntFlag{
			EnvVar: "BHYVE_CPUS",
			Name:   "bhyve-cpus",
			Usage:  "Number of CPUs in VM (default: 1)",
			Value:  defaultCPUCount,
		},
		mcnflag.IntFlag{
			Name:   "bhyve-engine-port",
			Usage:  "Docker engine port (default: 2376)",
			Value:  engine.DefaultPort,
			EnvVar: "BHYVE_ENGINE_PORT",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-ip-address",
			Usage:  "IP Address of machine (default: obtained via DHCP)",
			EnvVar: "BHYVE_IP_ADDRESS",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-mac-address",
			Usage:  "MAC Address of machine (default: generated)",
			EnvVar: "BHYVE_IP_ADDRESS",
		},
		mcnflag.StringFlag{
			Name:   "bhyve-ssh-user",
			Usage:  "SSH user (default: docker)",
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

func (d *Driver) getIPfromDHCPLease() (string, error) {
	leasefile := "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/bhyve.leases"

	file, err := os.Open(leasefile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if err := scanner.Err(); err != nil {
		return "", err
	}
	for scanner.Scan() {
		line := scanner.Text()
		// log.Debugf(line)
		if strings.Contains(line, d.MACAddress) {
			log.Debugf("Found our MAC")
			words := strings.Fields(line)
			log.Debugf("IP is: " + words[2])
			d.IPAddress = words[2]
			// easyCmd("ping", "-c", "1", d.IPAddress)
			return d.IPAddress, nil
		}
	}
	/*	for maxtries := 0; maxtries < retrycount; maxtries++ {
			time.Sleep(5000 * time.Millisecond)
		}
	*/
	return "", errors.New("IP Not Found")
}

func (d *Driver) waitForIP() error {
	var ip string
	var err error

	log.Infof("Waiting for VM to come online...")
	for i := 1; i <= 60; i++ {
		ip, err = d.getIPfromDHCPLease()
		if err != nil {
			log.Debugf("Not there yet %d/%d, error: %s", i, 60, err)
			time.Sleep(2 * time.Second)
			continue
		}

		if ip != "" {
			log.Debugf("Got an ip: %s", ip)
			d.IPAddress = ip

			break
		}
	}

	if ip == "" {
		return fmt.Errorf("Machine didn't return an IP after 120 seconds, aborting")
	}

	// Wait for SSH over NAT to be available before returning to user
	if err := drivers.WaitForSSH(d); err != nil {
		return err
	}

	return nil
}

func (d *Driver) GetIP() (string, error) {
	log.Debugf("GetIP called")
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
	return d.getIPfromDHCPLease()
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

	if fileExists("/dev/vmm/" + vmname) {
		// Breaks startup
		/*
			address := net.JoinHostPort(d.IPAddress, strconv.Itoa(d.SSHPort))

			_, err = net.DialTimeout("tcp", address, defaultTimeout)
			if err != nil {
				log.Debugf("STATE: stopped")
				return state.Error, nil
			}
		*/
		log.Debugf("STATE: running")
		return state.Running, nil
	}
	return state.Stopped, nil
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

	d.IPAddress = ""
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

	/*
		log.Debugf("Setting MAC address to", flags.String("bhyve-mac-address"))
	*/
	mac, _ := generatemacaddr()
	log.Debugf("Setting MAC address to", mac)
	d.MACAddress = mac

	d.SSHUser = "docker"
	log.Debugf("Setting port to", flags.String("bhyve-ssh-port"))
	d.SSHPort = flags.Int("bhyve-ssh-port")

	return nil
}

func (d *Driver) Start() error {
	log.Debugf("Start called")

	vmpath := d.ResolveStorePath("guest.img")
	log.Debugf("vmpath: %s", vmpath)
	bhyvelogpath := d.ResolveStorePath("bhyve.log")
	log.Debugf("bhyvelogpath: %s", bhyvelogpath)

	vmname, err := d.getBhyveVMName()
	if err != nil {
		return err
	}

	err = d.runGrub()
	if err != nil {
		return err
	}

	nmdmdev, err := findnmdmdev()
	macaddr := d.MACAddress
	tapdev, err := findtapdev()
	d.NetDev = tapdev
	cdpath, err := findcdpath()
	cpucount := strconv.Itoa(int(d.CPUcount))
	ram := strconv.Itoa(int(d.MemSize))
	log.Debugf("RAM size: " + ram)

	// cmd = exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-o", bhyvelogpath, "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
	cmd := exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s",
		"0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net,"+tapdev+",mac="+macaddr, "-s", "3:0,virtio-blk,"+
			vmpath, "-s", "4:0,virtio-rnd,/dev/random", "-s", "5:0,ahci-cd,"+cdpath, "-l", "com1,"+nmdmdev, "-c", cpucount, "-m", ram+"M",
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

	/*
		log.Debugf("Waiting for VM to boot")
		time.Sleep(60 * time.Second) // give VM 30 seconds to boot
	*/

	err = easyCmd("cp", "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/id_rsa",
		d.ResolveStorePath("/id_rsa"))
	if err != nil {
		return err
	}

	if err := d.waitForIP(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Stop() error {
	log.Debugf("Stop called")
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

	d.IPAddress = ""
	return nil
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
		DiskSize:   defaultDiskSize,
		MemSize:    defaultMemSize,
		CPUcount:   defaultCPUCount,
		MACAddress: "",
	}
}
