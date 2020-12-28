# Options for bhyve networking

Starting with: [this general page](http://empt1e.blogspot.com/2016/10/bhyve-networking-options.html) and 
[this doc from vm-bhyve](https://github.com/churchers/vm-bhyve/wiki/Virtual-Network-Interfaces)

## Basically you have these options:

* [bridge](https://www.freebsd.org/doc/handbook/network-bridging.html) based things
* [NIC Passthrough](https://wiki.freebsd.org/bhyve/pci_passthru)
* [Netgraph](https://www.freebsd.org/cgi/man.cgi?netgraph(4)) based things
** More on Netgraph [here](https://people.freebsd.org/~julian/netgraph.html) and 
[here](https://reviews.freebsd.org/D24620)




