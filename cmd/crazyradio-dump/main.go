// This utility dumps the Flash contents of the flie.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	//"time"

	"github.com/samofly/crazyradio"
	"github.com/samofly/crazyradio/usb"
)

var output = flag.String("output", "cflie.dump", "Output file")

const BootloaderChannel = 110

const (
	CMD_GET_INFO     = 0x10
	CMD_LOAD_BUFFER  = 0x14
	CMD_READ_BUFFER  = 0x15
	CMD_WRITE_FLASH  = 0x18
	CMD_FLASH_STATUS = 0x19
	CMD_READ_FLASH   = 0x1C
)

func main() {
	flag.Parse()

	got := make(map[int]bool)
	mem := make([]byte, 128*1024)
	buf := make([]byte, 128)

	list, err := usb.ListDevices()
	if err != nil {
		log.Fatalf("Unable list Crazyradio dongles: %v", err)
	}

	if len(list) == 0 {
		log.Fatalf("No Crazyradio USB dongles found")
	}

	info := list[0]
	dev, err := usb.Open(info)
	if err != nil {
		log.Fatalf("Unable to open Crazyradio USB dongle: %v", err)
	}
	defer dev.Close()

	err = dev.SetRateAndChannel(crazyradio.DATA_RATE_2M, BootloaderChannel)
	if err != nil {
		log.Fatal("SetRateAndChannel: %v", err)
	}
	for {
		_, err = dev.Write([]byte{0xFF, 0xFF, 0x10})
		if err != nil {
			log.Printf("write: %v", err)
			continue
		}
		n, err := dev.Read(buf)
		if err != nil {
			log.Printf("read: n: %d, err: %v", n, err)
			continue
		}
		if n == 1 {
			if buf[0] == 0 {
				// Empty packet, compact log
				fmt.Fprintf(os.Stderr, ".")
				continue
			}
			log.Printf("Strange packet: %v", buf[:n])
			continue
		}
		log.Printf("Packet: %v", buf[:n])
		// We're connected!
		break
	}

	readFlash := func(page uint16, offset uint16) []byte {
		return []byte{0xFF, 0xFF, CMD_READ_FLASH,
			byte(page & 0xFF), byte((page >> 8) & 0xFF),
			byte(offset & 0xFF), byte((offset >> 8) & 0xFF)}
	}
	for try := 0; try < 10; try++ {
		for page := uint16(0); page < 128; page++ {
			if try == 0 {
				fmt.Fprintf(os.Stderr, ".")
			}
			for offset := uint16(0); offset < 1024; offset += 16 {
				start := int(page)*1024 + int(offset)
				if got[start] {
					// Do not request already received chunks
					continue
				}
				if try > 0 {
					fmt.Fprintf(os.Stderr, "{Retry: %d}", start)
				}
				_, err = dev.Write(readFlash(page, offset))
				if err != nil {
					log.Printf("write: %v", err)
					continue
				}
				n, err := dev.Read(buf)
				if err != nil {
					log.Printf("read: n: %d, err: %v", n, err)
					continue
				}
				if n == 0 {
					log.Printf("Empty packet")
					continue
				}
				p := buf[1:n]
				// log.Printf("Packet: %v", p)
				if len(p) > 10 && p[2] == CMD_READ_FLASH {
					page := uint16(p[3]) + (uint16(p[4]) << 8)
					offset := uint16(p[5]) + (uint16(p[6]) << 8)
					data := p[7 : 7+16]
					// log.Printf("ReadFlashResponse: page: %d, offset: %d, data: %v", page, offset, data)
					start := int(page)*1024 + int(offset)
					copy(mem[start:start+16], data)
					got[start] = true
				}
			}
		}
	}

	missing := false
	for page := uint16(0); page < 128; page++ {
		for offset := uint16(0); offset < 1024; offset += 16 {
			start := int(page)*1024 + int(offset)
			if !got[start] {
				log.Printf("Missing chunk: start=%d", start)
				missing = true
			}
		}
	}
	if missing {
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\n")
	if err = ioutil.WriteFile(*output, mem, 0644); err != nil {
		log.Fatalf("Unable to dump memory to file %s: %v", *output, err)
	}
	log.Printf("OK - Memory dump saved to %s", *output)
}