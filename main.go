package main

import (
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
	return nil
}

func (d *Driver) GetIP() (string, error) {
	log.Debugf("GetIP called")
	return "10.0.1.117", nil
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
	return nil
}

func (d *Driver) PreCreateCheck() error {
	log.Debugf("PreCreateCheck called")
	return nil
}

func (d *Driver) Remove() error {
	log.Debugf("Remove called")
	return nil
}

func (d *Driver) Restart() error {
	log.Debugf("Restart called")
	return nil
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	log.Debugf("SetConfigFromFlags called")
	return nil
}

func (d *Driver) Start() error {
	log.Debugf("Start called")
	return nil
}

func (d *Driver) Stop() error {
	log.Debugf("Stop called")
	return nil
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

func main() {
	log.Debugf("main called")
}
