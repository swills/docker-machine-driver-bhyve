package main

import (
	"github.com/tarm/serial"
	"log"
	"os"
)

func main() {
	serialport := os.Args[1]
	logfile := os.Args[2]

	c := &serial.Config{Name: serialport, Baud: 115200}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 128)
	for {
		n, err := s.Read(buf)
		if err != nil {
			log.Fatal(err)
		}
		f, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}

		_, err = f.Write(buf[:n])
		if err != nil {
			log.Fatal(err)
		}

		f.Close()
	}
}
