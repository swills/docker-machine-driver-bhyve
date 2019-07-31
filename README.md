# What is this

This is a [Docker Machine](https://docs.docker.com/machine/overview/) Driver for [Bhyve](http://bhyve.org/). It is
heavily inspired by both the [xhyve driver](https://github.com/machine-drivers/docker-machine-driver-xhyve) and the [VirtualBox](https://github.com/docker/machine/tree/master/drivers/virtualbox) driver.

# How To Use It

```
docker-machine create --bhyve-ip-address 10.0.1.119
eval $(docker-machine env)
docker run --rm hello-world
```

###TODO

* Remove hard coded stuff
    * Paths
    * Files
    * Device names
    * MAC Address
    * CPU Count
    * Memory Size
    * `nmdm` Device
    * `tap` device
    * Docker port
    * `sudo` - may want to use `doas`
    * Avoid shelling out as much as possible

* Fetch ISO
* Log console
* Fix removing VM
* Fix state
* Implement unimplemented funcs
* Start vs. Create
* Stop
