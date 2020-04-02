package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"log"
	"net"
	"time"
)

// Listener runs a loop to read
func (eng *ChatEngine) Listener(ctx context.Context) error {
	const maxBufferSize = 4096

	listenAddress, err := net.ResolveUDPAddr("udp", ":"+eng.Me.Port)
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
			b := make([]byte, maxBufferSize)
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, addr, errRead := conn.ReadFrom(b) // blocking read
			if errRead == nil {
				go processData(b, addr.String(), eng.queue)
			} else {
				//log.Println(errRead) // expect io timeout error
			}
		}
	}

	log.Println("exiting listener")
	return err
}

// transform []byte to Message
func processData(b []byte, addr string, msgQueue chan *Message) {
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
