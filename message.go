package main

import (
	"fmt"
)

// Message always uses AES256 and HMAC-SHA256.
type Message struct {
	Payload   []byte // chat request/response/text
	Signature []byte // HMAC-SHA256 hash
	Type      PayloadType
}

func NewMessage(plType PayloadType, plaintext []byte, sharedKey []byte) (m *Message, err error) {
	// encrypt message with AEP
	ciphertext, err := AESEncrypt(plaintext, sharedKey)
	if err != nil {
		return
	}

	// sign with HMAC-SHA256 and shared key
	signature := SignHS256(ciphertext, sharedKey)

	m = &Message{
		Payload:   ciphertext,
		Signature: signature,
		Type:      plType,
	}
	return
}

func (m *Message) Verify(sharedKey []byte) (valid bool) {
	return ValidSignatureHS256(m.Signature, m.Payload, sharedKey)
}

func (m *Message) Decrypt(sharedKey []byte) (plaintext []byte, err error) {
	return AESDecrypt(m.Payload, sharedKey)
}

type PayloadType byte

const (
	PayloadText PayloadType = iota
	PayloadRequest
	PayloadResponse
)

// attempts to decrypt and decode the Message into a Request.
func (m *Message) GetRequest() (req *Request, err error) {
	if !m.Verify(ZeroKey) {
		err = fmt.Errorf("invalid signature")
		return
	}

	data, err := m.Decrypt(ZeroKey)
	if err != nil {
		return
	}

	req, ok := gobDecode(data, m.Type).(*Request)
	if !ok {
		err = fmt.Errorf("message type wasn't Request")
		return
	}

	return
}

// attempts to decrypt and decode the Message into a Response.
// SharedKey remains encrypted.
func (m *Message) GetResponse() (resp *Response, err error) {
	if !m.Verify(ZeroKey) {
		err = fmt.Errorf("invalid signature")
		return
	}

	data, err := m.Decrypt(ZeroKey)
	if err != nil {
		return
	}

	resp, ok := gobDecode(data, m.Type).(*Response)
	if !ok {
		err = fmt.Errorf("message type wasn't Response")
		return
	}

	return
}

// attempts to decrypt and decode the Message into a Text (using shared key).
func (m *Message) GetText(sharedKey []byte) (t *Text, err error) {
	if !m.Verify(sharedKey) {
		err = fmt.Errorf("invalid signature")
		return
	}

	data, err := m.Decrypt(sharedKey)
	if err != nil {
		return
	}

	t, ok := gobDecode(data, m.Type).(*Text)
	if !ok {
		err = fmt.Errorf("message type wasn't Text")
		return
	}

	return
}

// make it easier to make a Message from Request
func PackageRequest(req *Request) (m *Message, err error) {
	data, err := gobEncode(req)
	if err != nil {
		return
	}

	m, err = NewMessage(PayloadRequest, data, ZeroKey)
	if err != nil {
		return
	}

	return
}

// make it easier to make a Message from Response
func PackageResponse(resp *Response) (m *Message, err error) {
	data, err := gobEncode(resp)
	if err != nil {
		return
	}

	m, err = NewMessage(PayloadResponse, data, ZeroKey)
	if err != nil {
		return
	}

	return
}

// make it easier to make a Message from Text
func PackageText(t *Text, sharedKey []byte) (m *Message, err error) {
	data, err := gobEncode(t)
	if err != nil {
		return
	}

	m, err = NewMessage(PayloadText, data, sharedKey)
	if err != nil {
		return
	}

	return
}
