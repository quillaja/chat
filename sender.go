package main

import (
	"bytes"
	"encoding/gob"
	"net"
)

// to is full address (ex 111.222.333.444:555)
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

	return nil
}
