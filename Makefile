bin/docker-machine-driver-bhyve: main.go
	go build -o bin/docker-machine-driver-bhyve main.go

clean:
	rm bin/docker-machine-driver-bhyve
