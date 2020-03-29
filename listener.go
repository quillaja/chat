package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"log"
	"net"
)

// port without :
func Listener(ctx context.Context, port string, msgQueue chan *Message) error {
	const maxBufferSize = 4096

	listenAddress, err := net.ResolveUDPAddr("udp", ":"+port)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", listenAddress)
	defer conn.Close()
	if err != nil {
		return err
	}

	var done bool
	for !done {
		select {
		case <-ctx.Done():
			done = true // exit for loop

		default:
			buf := make([]byte, maxBufferSize)
			_, _, errRead := conn.ReadFrom(buf)
			if errRead == nil {
				// log.Printf("read %d bytes fron udp\n", n)
				go processData(buf, msgQueue)
			} else {
				log.Println(err)
			}
		}
	}

	log.Println("exiting listener")
	return err
}

// transform []byte to Message
func processData(b []byte, msgQueue chan *Message) {
	var m Message

	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&m)

	if err != nil {
		log.Println(err)
		return
	}

	msgQueue <- &m
}
