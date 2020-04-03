package main

import (
	"bytes"
	"encoding/gob"
)

//
// These 2 functions provide an slightly less tedious way to encode/decode
// the 3 primary datatypes used by the ChatEngine.
//

func gobDecode(b []byte, plType PayloadType) interface{} {

	dec := func() *gob.Decoder {
		return gob.NewDecoder(bytes.NewBuffer(b))
	}

	// need one of these "blocks" for each type to decode

	switch plType {
	case PayloadText:
		x := &Text{}
		if dec().Decode(x) == nil {
			return x
		}

	case PayloadRequest:
		x := &Request{}
		if dec().Decode(x) == nil {
			return x
		}

	case PayloadResponse:
		x := &Response{}
		if dec().Decode(x) == nil {
			return x
		}
	}

	return nil
}

func gobEncode(v interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}
