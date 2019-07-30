package bhyve

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
)

const (
	defaultTimeout = 15 * time.Second
)

type Driver struct {
	*drivers.BaseDriver
	EnginePort int
}

func (d *Driver) Create() error {
	log.Debugf("Create called")
	ip, err := d.GetIP()
	if err != nil {
		d.IPAddress = ip
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
	return "10.0.1.118", nil
}

func (d *Driver) GetSSHHostname() (string, error) {
	log.Debugf("GetSSHHostname called")
	return d.GetIP()
}

func (d *Driver) GetState() (state.State, error) {
	log.Debugf("GetState called")
	address := net.JoinHostPort(d.IPAddress, strconv.Itoa(d.SSHPort))

	_, err := net.DialTimeout("tcp", address, defaultTimeout)
	if err != nil {
		return state.Stopped, nil
	}

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
	return errors.New("PreCreateCheck not implemented yet")
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
	return errors.New("SetConfigFromFlags not implemented yet")
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
	}
}
