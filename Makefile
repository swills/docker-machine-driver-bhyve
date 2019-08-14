bin/docker-machine-driver-bhyve: main.go
	go build -ldflags="-s -w" -o docker-machine-driver-bhyve main.go
	go build -ldflags="-s -w" nmdm.go

clean:
	rm -f docker-machine-driver-bhyve nmdm
