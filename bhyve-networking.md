# Options for bhyve networking

Starting with these pages:
* [this general page](http://empt1e.blogspot.com/2016/10/bhyve-networking-options.html) 
* [this doc from vm-bhyve](https://github.com/churchers/vm-bhyve/wiki/Virtual-Network-Interfaces)
* [personal notes](https://blog.grem.de/pages/ayvn.html)

## Basically you have these options:

* [bridge](https://www.freebsd.org/doc/handbook/network-bridging.html) based things. May be used with:
  * [tap](https://www.freebsd.org/cgi/man.cgi?query=tap&apropos=0&sektion=4&manpath=FreeBSD+12.2-RELEASE+and+Ports&arch=default&format=html)
  * [epair](https://www.freebsd.org/cgi/man.cgi?query=epair&sektion=4)
  * [ng_nat](https://www.freebsd.org/cgi/man.cgi?query=ng_nat&apropos=0&sektion=4&manpath=FreeBSD+12.2-RELEASE+and+Ports&arch=default&format=html)
* [vxlan](https://www.bsdcan.org/2016/schedule/events/715.en.html)
* [Open vSwtich](https://docs.openvswitch.org/en/latest/intro/install/general/)
* [NIC Passthrough](https://wiki.freebsd.org/bhyve/pci_passthru)
* [Netgraph](https://www.freebsd.org/cgi/man.cgi?netgraph(4)) based things
  * More on Netgraph [here](https://people.freebsd.org/~julian/netgraph.html) and [here](https://reviews.freebsd.org/D24620)
  * There's also [ng_bridge](https://www.freebsd.org/cgi/man.cgi?query=ng_bridge&sektion=4) which is not the same as if_bridge above
  * [this script](https://github.com/freebsd/freebsd/blob/master/share/examples/netgraph/virtual.lan) might help?

Compare and contrast with [VirtualBox Networking](https://www.virtualbox.org/manual/ch06.html)
