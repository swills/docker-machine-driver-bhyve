// Copyright 2019 Steve Wills. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bhyve

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/docker/machine/libmachine/log"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	if err != nil {
		return err
	}

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
			_ = easyCmd("sudo", "bhyvectl", "--destroy", "--vm="+vmname)
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

func runGrub(devmap string, memsize string, vmname string) error {
	for maxtries := 0; maxtries < retrycount; maxtries++ {
		cmd := exec.Command("sudo", "/usr/local/sbin/grub-bhyve", "-m", devmap, "-r", "cd0", "-M",
			memsize+"M", vmname)
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

func writeDHCPConf(dhcpconffile string, bridge string, dhcprange string) error {
	log.Debugf("Writing DHCP server config")

	f, err := os.OpenFile(dhcpconffile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString("port=0\ndomain-needed\nno-resolv\nexcept-interface=lo0\nbind-interfaces\nlocal-service\ndhcp-authoritative\n\n")
	if err != nil {
		return err
	}

	_, err = f.WriteString("interface=" + bridge + "\n")
	if err != nil {
		return err
	}

	_, err = f.WriteString("dhcp-range=" + dhcprange + "\n")
	if err != nil {
		return err
	}

	return nil
}

func startDHCPServer(dhcpdir string, bridge string, dhcprange string) error {
	log.Debugf("Starting DHCP Server")

	dhcppidfile := filepath.Join(dhcpdir, "dnsmasq.pid")
	dhcpconffile := filepath.Join(dhcpdir, "dnsmasq.conf")
	dhcpleasefile := filepath.Join(dhcpdir, "bhyve.leases")

	if !fileExists(dhcpconffile) {
		err := writeDHCPConf(dhcpconffile, bridge, dhcprange)
		if err != nil {
			return err
		}
	}
	// dnsmasq may leave it's PID file if killed?
	if !fileExists(dhcppidfile) {
		err := easyCmd("sudo", "dnsmasq", "-i", bridge, "-C", dhcpconffile, "-x", dhcppidfile, "-l", dhcpleasefile)
		if err != nil {
			return err
		}
	}
	return nil
}

func findtapdev(bridge string) (string, error) {
	lasttap := 0
	numtaps := 0
	nexttap := 0
	ifaces, _ := net.Interfaces()
	r1 := regexp.MustCompile("^tap")
	for _, iface := range ifaces {
		log.Debugf("Checking interface %s", iface.Name)
		match := r1.MatchString(iface.Name)
		if match {
			r2 := regexp.MustCompile(`tap(?P<num>\d*)`)
			res := r2.FindAllStringSubmatch(iface.Name, -1)
			tapnum, err := strconv.Atoi(res[0][1])
			if err != nil {
				return "", err
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

	err = easyCmd("sudo", "ifconfig", bridge, "addm", nexttapname)
	if err != nil {
		return "", err
	}

	err = easyCmd("sudo", "ifconfig", nexttapname, "up")
	if err != nil {
		return "", err
	}

	return nexttapname, nil
}

func getIPfromDHCPLease(dhcpleasefile string, macaddress string) (string, error) {
	file, err := os.Open(dhcpleasefile)
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
		if strings.Contains(line, macaddress) {
			log.Debugf("Found our MAC")
			words := strings.Fields(line)
			ip := words[2]
			log.Debugf("IP is: " + ip)
			return ip, nil
		}
	}

	return "", errors.New("IP Not Found")
}

func setupnet(bridge string, subnet string) error {
	localhost := "127.0.0.0/8"
	_, localhostsubnet, _ := net.ParseCIDR(localhost)

	_, oursubnet, _ := net.ParseCIDR(subnet)

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		log.Debugf("Checking interface %s", iface.Name)

		if iface.Name == bridge {
			log.Debugf("Interface %s exists, assuming network setup properly", bridge)
			return nil
		}
	}

	found := false
	useip := net.IP{}
	useiface := net.Interface{}
	for _, iface := range ifaces {
		if found {
			break
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipAddr, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				log.Debugf("Failed to parse address %s", addr.String())
				continue
			}
			theip := net.IP.To4(ipAddr)
			if theip == nil {
				continue
			}
			if !localhostsubnet.Contains(theip) && !oursubnet.Contains(theip) {
				log.Debugf("Interface %s has address %s and looks like a good candidate", iface.Name, theip.String())
				found = true
				useip = theip
				useiface = iface
			}
		}
	}

	log.Debugf("Setting up %s on %s, aliased to %s on %s", subnet, bridge, useip, useiface.Name)

	err := easyCmd("sudo", "ifconfig", bridge, "create")
	if err != nil {
		return err
	}
	err = easyCmd("sudo", "ifconfig", bridge, subnet)
	if err != nil {
		return err
	}
	err = easyCmd("sudo", "ifconfig", bridge, "up")
	if err != nil {
		return err
	}

	err = easyCmd("sudo", "ngctl", "mkpeer", useiface.Name+":", "nat", "lower", "in")
	if err != nil {
		return err
	}

	err = easyCmd("sudo", "ngctl", "name", useiface.Name+":lower", useiface.Name+"_NAT")
	if err != nil {
		return err
	}
	err = easyCmd("sudo", "ngctl", "connect", useiface.Name+":", useiface.Name+"_NAT:", "upper", "out")
	if err != nil {
		return err
	}

	err = easyCmd("sudo", "ngctl", "msg", useiface.Name+"_NAT:", "setdlt", "1")
	if err != nil {
		return err
	}

	err = easyCmd("sudo", "ngctl", "msg", useiface.Name+"_NAT:", "setaliasaddr", useip.String())
	if err != nil {
		return err
	}

	// sudo ngctl msg igb0_NAT: redirectport '{alias_addr=10.0.1.8 alias_port=12377 local_addr=192.168.8.73 local_port=2376 proto=6}'
	return nil
}

func startConsoleLogger(storepath string, nmdmdev string) error {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))

	if err != nil {
		return err
	}

	err = easyCmd("/usr/sbin/daemon", "-f", "-p",
		filepath.Join(storepath, "nmdm.pid"), dir+"/nmdm", nmdmdev+"B",
		filepath.Join(storepath, "console.log"))
	if err != nil {
		return err
	}

	return nil
}

func checkRequiredCommand(commandname string) error {
	_, err := exec.LookPath(commandname)
	if err != nil {
		return err
	}
	return nil
}

func checkRequiredCommands() error {
	if err := checkRequiredCommand("sudo"); err != nil {
		return errors.New("sudo not installed")
	}
	if err := checkRequiredCommand("grub2-bhyve"); err != nil {
		return errors.New("grub2-bhyve not installed")
	}
	if err := checkRequiredCommand("dnsmasq"); err != nil {
		return errors.New("dnsmasq not installed")
	}
	return nil
}

func kmodLoaded(kmod string) error {
	cmd := exec.Command("kldstat", "-m", kmod)
	err := cmd.Start()
	if err != nil {
		log.Debugf("kmod %s is loaded", kmod)
		return nil

	}
	return err
}

func checkRequireKmods() error {
	err := kmodLoaded("vmm")
	if err != nil {
		return err
	}

	err = kmodLoaded("nmdm")
	if err != nil {
		return err
	}

	err = kmodLoaded("ng_ether")
	if err != nil {
		return err
	}

	return nil
}
