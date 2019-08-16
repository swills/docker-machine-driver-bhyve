# What is this

This is a [Docker Machine](https://docs.docker.com/machine/overview/) Driver for [Bhyve](http://bhyve.org/). It is
heavily inspired by the [xhyve driver](https://github.com/machine-drivers/docker-machine-driver-xhyve), the
[generic](https://github.com/docker/machine/tree/master/drivers/generic) driver and the
[VirtualBox](https://github.com/docker/machine/tree/master/drivers/virtualbox) driver.
See also [this issue](https://github.com/machine-drivers/docker-machine-driver-xhyve/issues/200).

# Requirements

You must be running a version of [FreeBSD](https://www.FreeBSD.org/) which includes [this](https://svnweb.freebsd.org/base?view=revision&revision=342168) [commit](https://github.com/freebsd/freebsd/commit/53dba18a1b398c13a795558d636b8dce20ef376f). As of now (2019/08/16), this is only in FreeBSD-CURRENT.

# How To Use It

## One time setup

* Install required packages:
  * `sudo`
  * `grub2-bhyve`
  * `dnsmasq`

* User running docker-machine must have password-less `sudo` access:
 
```
echo 'jsmith ALL=(ALL) NOPASSWD: ALL' >> /usr/local/etc/sudoers
```

* Add user to wheel group:

```
pw groupmod wheel -m jsmith
```

* Add these lines to /etc/devfs.rules:

```
[system=10]
add path 'nmdm*' mode 0660
```

* Set `devfs_system_ruleset="system"` in `/etc/rc.conf` and run `service devfs restart`

* Add `ng_ether`, `nmdm` and `vmm` to `kld_list` in `/etc/rc.conf`, `kldload ng_ether`, `kldload vmm`, `kldload nmdm`.

## Build

```
make
```

## Setup

```
export MACHINE_DRIVER=bhyve
export PATH=${PATH}:${PWD}
```

## Normal usage

```
docker-machine create
eval $(docker-machine env)
docker run --rm hello-world
```
