package main

import (
	"bytes"
	"encoding/gob"
	"net"
	"time"
)

// Send a Message. `to` is full address (ex 111.222.333.444:555).
func Send(to string, msg *Message) error {
	conn, err := net.Dial("udp", to)
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err = enc.Encode(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	// TODO: experiment to detect when a Send fails due to
	// there being no one on the other side.
	// SHOULD timeout most of the time. Since send is on a random ephemeral
	// port, I would not expect to ever accidently read data meant to be
	// read by "Listener()".
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = conn.Read([]byte{0})
	if neterr, ok := err.(net.Error); ok {
		if !neterr.Timeout() { // timeout error is expected
			return err
		}
	}

	return nil
}
