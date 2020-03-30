package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"log"
	"net"
	"time"
)

// port without :
func (app *Application) Listener(ctx context.Context) error {
	const maxBufferSize = 4096

	listenAddress, err := net.ResolveUDPAddr("udp", ":"+app.Me.Port)
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
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, addr, errRead := conn.ReadFrom(buf) // blocking read
			if errRead == nil {
				go processData(buf, addr.String(), app.queue)
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
