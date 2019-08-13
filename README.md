# What is this

This is a [Docker Machine](https://docs.docker.com/machine/overview/) Driver for [Bhyve](http://bhyve.org/). It is
heavily inspired by the [xhyve driver](https://github.com/machine-drivers/docker-machine-driver-xhyve), the
[generic](https://github.com/docker/machine/tree/master/drivers/generic) driver and the
[VirtualBox](https://github.com/docker/machine/tree/master/drivers/virtualbox) driver.
See also [this issue](https://github.com/machine-drivers/docker-machine-driver-xhyve/issues/200).

# How To Use It

* Must have `sudo` installed and user running docker-machine must have password-less `sudo` access.
* Interface `bridge0` must exist and must have a member with a DHCP server on the same network

## One time setup

### Setup devfs

Add user to wheel group, then add these lines to /etc/devfs.rules:

```
[system=10]
add path 'nmdm*' mode 0660
```

Set `devfs_system_ruleset="system"` in `/etc/rc.conf` and run `service devfs restart`

### Setup bhyve

Add `ng_ether`, `nmdm` and `vmm` to `kld_list` in `/etc/rc.conf`, `kldload ng_ether`, `kldload vmm`, `kldload nmdm`.

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
docker-machine kill default || :
docker-machine rm -y default || :

docker-machine create
eval $(docker-machine env)
docker run --rm hello-world
```


### TODO

* Remove reliance on external files
* Remove reliance on external config
* Remove hard coded stuff
    * Paths
    * Files
    * Docker port
    * `sudo` - may want to use `doas`
* Avoid shelling out as much as possible

* Fetch ISO
* Log console
* Manage processes (grub-bhyve, bhyve, serial logger)
* Networking
    * Create VLAN
    * Run DHCP server
    * Attach VLAN to bridge
    * Attach machines to VLAN
    * Get IP from DHCP server
