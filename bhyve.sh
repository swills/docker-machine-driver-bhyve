#!/bin/sh

sudo bhyvectl --destroy --vm=boot2docker
rm guest.img
truncate -s 16g guest.img
dd if=userdata.tar of=guest.img conv=notrunc status=none
cat grub.txt | sudo grub-bhyve -m device.map -r cd0 -M 1024M boot2docker
sudo daemon -f bhyve -A -H -P \
	-s 0:0,hostbridge \
	-s 1:0,lpc \
	-s 2:0,virtio-net,tap0,mac=00:A0:98:00:00:01 \
	-s 3:0,virtio-blk,./guest.img \
	-s 4:0,ahci-cd,./boot2docker.iso \
	-l com1,/dev/nmdm0A \
	-c 2 \
	-m 1024M boot2docker
