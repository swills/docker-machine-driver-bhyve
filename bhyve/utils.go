// Copyright 2019 Steve Wills. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bhyve

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/docker/machine/libmachine/log"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func generateMACAddress() string {
	oidprefix := "58:9c:fc"
	b1, _ := randomHex(1)
	b2, _ := randomHex(1)
	b3, _ := randomHex(1)
	return oidprefix + ":" + b1 + ":" + b2 + ":" + b3
}

func stripCtlAndExtFromBytes(str string) string {
	// https://rosettacode.org/wiki/Strip_control_codes_and_extended_characters_from_a_string#Go
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

func randomHex(n int) (string, error) {
	// https://sosedoff.com/2014/12/15/generate-random-hex-string-in-go.html
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

func findNMDMDev() (string, error) {
	lastnmdm := 0

	for {
		nmdmdev := "/dev/nmdm" + strconv.Itoa(lastnmdm)
		log.Debugf("checking nmdm: %s", nmdmdev+"A")
		cmd := exec.Command("sudo", "fuser", nmdmdev+"A")
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
			return "", errors.New("could not find nmdm dev")
		}
		time.Sleep(1 * time.Second)
	}
}

func copyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}

	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}

	defer destination.Close()

	nBytes, err := io.Copy(destination, source)

	if err != nil {
		return nBytes, err
	}

	fi, err := os.Stat(src)
	if err != nil {
		return nBytes, err
	}

	if err := os.Chmod(dst, fi.Mode()); err != nil {
		return nBytes, err
	}

	return nBytes, nil
}

func ensureIPForwardingEnabled() error {
	log.Debugf("Checking IP forwarding")
	cmd := exec.Command("sysctl", "-n", "net.inet.ip.forwarding")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	isenabled, err := strconv.Atoi(strings.Trim(stdout.String(), "\n"))
	if err != nil {
		return err
	}

	if isenabled == 0 {
		log.Debugf("IP forwarding not enabled, enabling")
		err = easyCmd("sudo", "sysctl", "net.inet.ip.forwarding=1")
		if err != nil {
			return err
		}
	}
	return nil
}

func destroyTap(netdev string) error {
	return easyCmd("sudo", "ifconfig", netdev, "destroy")
}

func destroyVM(vmname string) error {
	tries := 0
	for ; tries < retrycount; tries++ {
		if fileExists("/dev/vmm/" + vmname) {
			err := easyCmd("sudo", "bhyvectl", "--destroy", "--vm="+vmname)
			if err != nil {
			}
			time.Sleep(sleeptime * time.Millisecond)
		}
	}

	if tries > retrycount {
		return fmt.Errorf("failed to kill %s", vmname)
	}

	return nil
}

func killConsoleLogger(pidfile string) error {
	nmdmpid, err := ioutil.ReadFile(pidfile)
	if err != nil {
		log.Debugf("Could not get pid file for console logger")
	}

	pid, err := strconv.Atoi(string(nmdmpid))
	if err != nil {
		log.Debugf("Failed to parse console logger pid")
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		log.Debugf("Couldn't find console logger process %s", nmdmpid)
		return err
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		return err
	}

	return nil
}

func writeDeviceMap(devmap string, cdpath string, diskname string) error {
	f, err := os.Create(devmap)
	if err != nil {
		return err
	}

	_, err = f.WriteString("(hd0) " + diskname + "\n")
	if err != nil {
		return err
	}
	_, err = f.WriteString("(cd0) " + cdpath + "\n")
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
