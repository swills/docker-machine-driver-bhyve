bin/docker-machine-driver-bhyve: main.go
	go build -o docker-machine-driver-bhyve main.go

clean:
	rm -f docker-machine-driver-bhyve
