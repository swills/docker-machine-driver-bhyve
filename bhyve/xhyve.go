// Copyright 2015 The docker-machine-driver-xhyve Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bhyve

import (
	"archive/tar"
	"bytes"
	"fmt"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnutils"
	"github.com/docker/machine/libmachine/ssh"
	"gitlab.mouf.net/swills/docker-machine-driver-bhyve/b2d"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (d *Driver) updateISOCache(isoURL string) error {
	b2dinstance := b2d.NewB2dUtils(d.StorePath)
	mcnutilsinstance := mcnutils.NewB2dUtils(d.StorePath)

	// recreate the cache dir if it has been manually deleted
	if _, err := os.Stat(b2dinstance.ImgCachePath); os.IsNotExist(err) {
		log.Infof("Image cache directory does not exist, creating it at %s...", b2dinstance.ImgCachePath)
		if err := os.Mkdir(b2dinstance.ImgCachePath, 0700); err != nil {
			return err
		}
	}

	// Check owner of storage cache directory
	cacheStat, _ := os.Stat(b2dinstance.ImgCachePath)

	if int(cacheStat.Sys().(*syscall.Stat_t).Uid) == 0 {
		log.Debugf("Fix %s directory permission...", cacheStat.Name())
		err := os.Chown(b2dinstance.ImgCachePath, syscall.Getuid(), syscall.Getegid())
		if err != nil {
			return err
		}
	}

	if isoURL != "" {
		// Non-default B2D are not cached
		log.Debugf("Not caching non-default B2D URL	")
		return nil
	}

	exists := b2dinstance.Exists()
	if !exists {
		log.Info("No default Boot2Docker ISO found locally, downloading the latest release...")
		return mcnutilsinstance.DownloadLatestBoot2Docker("")
	}

	latest := b2dinstance.IsLatest()
	if !latest {
		log.Info("Default Boot2Docker ISO is out-of-date, downloading the latest release...")
		return mcnutilsinstance.DownloadLatestBoot2Docker("")
	}

	return nil
}

func (d *Driver) copyIsoToMachineDir(isoURL, machineName string) error {
	b2dinst := b2d.NewB2dUtils(d.StorePath)
	mcnutilsinstance := mcnutils.NewB2dUtils(d.StorePath)

	if err := d.updateISOCache(isoURL); err != nil {
		return err
	}

	isoPath := filepath.Join(b2dinst.ImgCachePath, isoFilename)
	if isoStat, err := os.Stat(isoPath); err == nil {
		if int(isoStat.Sys().(*syscall.Stat_t).Uid) == 0 {
			log.Debugf("Fix %s file permission...", isoStat.Name())
			err = os.Chown(isoPath, syscall.Getuid(), syscall.Getegid())
			if err != nil {
				return err
			}
		}
	}

	machineDir := filepath.Join(d.StorePath, "machines", machineName)
	machineIsoPath := filepath.Join(machineDir, isoFilename)

	// By default just copy the existing "cached" iso to the machine's directory...
	defaultISO := filepath.Join(b2dinst.ImgCachePath, defaultISOFilename)
	if isoURL == "" {
		log.Infof("Copying %s to %s...", defaultISO, machineIsoPath)
		_, err := copyFile(defaultISO, machineIsoPath)
		return err
	}

	// if ISO is specified, check if it matches a github releases url or fallback to a direct download
	downloadURL, err := b2dinst.GetReleaseURL(isoURL)
	if err != nil {
		return err
	}

	return mcnutilsinstance.DownloadISO(machineDir, b2dinst.Filename(), downloadURL)
}

// Make a boot2docker userdata.tar key bundle
func (d *Driver) generateKeyBundle() (*bytes.Buffer, error) {
	magicString := "boot2docker, please format-me"

	log.Infof("Creating SSH key...")
	if err := ssh.GenerateSSHKey(d.GetSSHKeyPath()); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// magicString first so the automount script knows to format the disk
	file := &tar.Header{Name: magicString, Size: int64(len(magicString))}
	if err := tw.WriteHeader(file); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(magicString)); err != nil {
		return nil, err
	}
	// .ssh/key.pub => authorized_keys
	file = &tar.Header{Name: ".ssh", Typeflag: tar.TypeDir, Mode: 0700}
	if err := tw.WriteHeader(file); err != nil {
		return nil, err
	}
	pubKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
	if err != nil {
		return nil, err
	}
	file = &tar.Header{Name: ".ssh/authorized_keys", Size: int64(len(pubKey)), Mode: 0644}
	if err := tw.WriteHeader(file); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(pubKey)); err != nil {
		return nil, err
	}
	file = &tar.Header{Name: ".ssh/authorized_keys2", Size: int64(len(pubKey)), Mode: 0644}
	if err := tw.WriteHeader(file); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(pubKey)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

func (d *Driver) generateRawDiskImage(diskPath string, size int64) error {
	f, err := os.OpenFile(diskPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	f.Close()

	if err := os.Truncate(diskPath, d.DiskSize); err != nil {
		return err
	}

	tarBuf, err := d.generateKeyBundle()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(diskPath, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	file.Seek(0, os.SEEK_SET)
	_, err = file.Write(tarBuf.Bytes())
	if err != nil {
		return err
	}
	file.Close()

	return nil
}

func (d *Driver) waitForIP() error {
	var ip string
	var err error

	log.Infof("Waiting for VM to come online...")
	for i := 1; i <= 60; i++ {
		ip, err = getIPfromDHCPLease(filepath.Join(d.StorePath, "bhyve.leases"), d.MACAddress)
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
		return fmt.Errorf("machine didn't return an IP after 120 seconds, aborting")
	}

	// Wait for SSH over NAT to be available before returning to user
	if err := drivers.WaitForSSH(d); err != nil {
		return err
	}

	return nil
}
