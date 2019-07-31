package bhyve

import (
	"errors"
	"fmt"
	"time"
	"os/exec"
	"os"
	"io"
	"io/ioutil"
	"bytes"
	"strings"
	"strconv"
/*
	"net"
*/

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
)

const (
	defaultTimeout = 15 * time.Second
        defaultDiskSize       = 16384
)

type Driver struct {
	*drivers.BaseDriver
	EnginePort    int
        DiskSize      int64
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
        log.Debugf("EXEC: " + strings.Join(args," "))
        cmd:= exec.Command(args[0], args[1:]...)
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
	return "tap1", nil
}

func findcdpath() (string, error) {
	return "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/boot2docker.iso", nil
}

func (d *Driver) getMachineDir() (string, error) {
	return d.StorePath + "/machines/" + d.MachineName + "/", nil
}

func (d *Driver) Create() error {
	log.Debugf("Create called")

	log.Debugf("Removing VM %s", d.MachineName)
	err := easyCmd("sudo", "bhyvectl", "--destroy", "--vm=" + d.MachineName)
	// XXX error checking
        if err != nil {
        }
	vmpath := d.StorePath + "/machines/" + d.MachineName + "/" + "guest.img"
	bhyvelogpath := d.StorePath + "/machines/" + d.MachineName + "/" + "bhyve.log"
	log.Debugf("vmpath: %s", vmpath)
	log.Debugf("bhyvelogpath: %s", bhyvelogpath)

	log.Debugf("Deleting %s", vmpath)
	err = os.RemoveAll(vmpath)
	if err != nil {
		return err
	}

	err = easyCmd("truncate", "-s", strconv.Itoa(int(d.DiskSize)), vmpath)
	if err != nil {
		return err
	}

	err = easyCmd("dd", "if=/usr/home/swills/Documents/git/docker-machine-driver-bhyve/userdata.tar", "of=" + vmpath, "conv=notrunc", "status=none")
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "grub-bhyve", "-m", "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/device.map", "-r", "cd0", "-M", "1024M", d.MachineName)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, "linux (cd0)/boot/vmlinuz waitusb=5:LABEL=boot2docker-data base norestore noembed\n")
		io.WriteString(stdin, "initrd (cd0)/boot/initrd.img\n")
		io.WriteString(stdin, "boot\n")
	} ()

	out, err := cmd.CombinedOutput()
	log.Debugf("grub-bhyve: " + stripCtlAndExtFromBytes(string(out)))
	if err != nil {
		return err
	}

	nmdmdev, err := findnmdmdev()
	macaddr, err := findmacaddr()
	tapdev, err := findtapdev()
	cdpath, err := findcdpath()
	cpucount := "2"
	ram := "1024"

	log.Debugf("XXX: 1")
	// cmd = exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-o", bhyvelogpath, "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
	cmd = exec.Command("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
	log.Debugf("XXX: 2")
	stderr, err := cmd.StderrPipe()
	log.Debugf("XXX: 3")
	if err != nil {
		return err
	}

	log.Debugf("XXX: 4")
	if err := cmd.Start(); err != nil {
		return err
	}

	log.Debugf("XXX: 5")
	slurp, _ := ioutil.ReadAll(stderr)
	log.Debugf("XXX: 6")
	log.Debugf("%s\n", slurp)

	log.Debugf("XXX: 7")
	if err := cmd.Wait(); err != nil {
		return err
	}
	log.Debugf("XXX: 8")
	log.Debugf("bhyve: " + stripCtlAndExtFromBytes(string(out)))

/*
	err = easyCmd("/usr/sbin/daemon", "-t", "XXXXX", "-f", "sudo", "bhyve", "-A", "-H", "-P", "-s", "0:0,hostbridge", "-s", "1:0,lpc", "-s", "2:0,virtio-net," + tapdev + ",mac=" + macaddr, "-s", "3:0,virtio-blk," + vmpath, "-s", "4:0,ahci-cd," + cdpath, "-l", "com1," + nmdmdev, "-c", cpucount, "-m", ram + "M", d.MachineName)
	if err != nil {
		return err
	}
*/

	err = easyCmd("cp", "/usr/home/swills/Documents/git/docker-machine-driver-bhyve/id_rsa", d.StorePath + "/machines/" + d.MachineName + "/" + "id_rsa")
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
                        Name:   "bhyve-disk-size",
                        Usage:  "Size of disk for host in MB",
                        Value:  defaultDiskSize,
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
		mcnflag.IntFlag{
			Name:   "bhyve-ssh-port",
			Usage:  "SSH port",
			Value:  drivers.DefaultSSHPort,
			EnvVar: "BHYVE_SSH_PORT",
		},
	}
	return nil
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
/*
	address := net.JoinHostPort(d.IPAddress, strconv.Itoa(d.SSHPort))

	_, err := net.DialTimeout("tcp", address, defaultTimeout)
	if err != nil {
		return state.Stopped, nil
	}
*/

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
	return errors.New("Kill not implemented yet")
}

func (d *Driver) PreCreateCheck() error {
	log.Debugf("PreCreateCheck called")
/*
	return errors.New("PreCreateCheck not implemented yet")
*/
	return nil
}

func (d *Driver) Remove() error {
	log.Debugf("Remove called")
	return errors.New("Remove not implemented yet")
}

func (d *Driver) Restart() error {
	log.Debugf("Restart called")
	return errors.New("Restart not implemented yet")
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	log.Debugf("SetConfigFromFlags called")
        disksize := int64(flags.Int("bhyve-disk-size")) * 1024 * 1024
	log.Debugf("Setting disk size to %d", disksize)
        d.DiskSize = disksize
	log.Debugf("Setting ip address to", flags.String("bhyve-ip-address"))
        d.IPAddress= flags.String("bhyve-ip-address")
        d.SSHUser = "docker"
	return nil
}

func (d *Driver) Start() error {
	log.Debugf("Start called")
	return errors.New("Start not implemented yet")
}

func (d *Driver) Stop() error {
	log.Debugf("Stop called")
	return errors.New("Stop not implemented yet")
}

func NewDriver(hostName, storePath string) *Driver {
	log.Debugf("NewDriver called")
	return &Driver{
		EnginePort: engine.DefaultPort,
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
                        StorePath:   storePath,
		},
                DiskSize:       defaultDiskSize,
	}
}
