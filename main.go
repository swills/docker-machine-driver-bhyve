package main

import (
        "github.com/docker/machine/libmachine/drivers/plugin"
        "gitlab.mouf.net/swills/docker-machine-driver-bhyve/bhyve"
)

func main() {
        plugin.RegisterDriver(new(bhyve.Driver))
}

