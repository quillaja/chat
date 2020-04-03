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

// NewMessage creates a new encrypted and signed Message using the provided
// plaintext payload and shared key.
//
// Encryption is done using AES256 in cipher block chaining (CBC) mode, and
// signing is done using HMAC-SHA256. Shared key should be 32 bytes to do
// AES256. Use GenerateAES256Key() to do so.
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

// Verify the Signature on a message.
func (m *Message) Verify(sharedKey []byte) (valid bool) {
	return ValidSignatureHS256(m.Signature, m.Payload, sharedKey)
}

// Decrypt a message payload into plaintext bytes.
func (m *Message) Decrypt(sharedKey []byte) (plaintext []byte, err error) {
	return AESDecrypt(m.Payload, sharedKey)
}

// PayloadType indicates the type encrypted in a Message.
type PayloadType byte

// Values of PayloadType
const (
	PayloadText PayloadType = iota
	PayloadRequest
	PayloadResponse
)

// GetRequest attempts to decrypt and decode the Message into a Request.
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

// GetResponse attempts to decrypt and decode the Message into a Response.
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

// GetText attempts to decrypt and decode the Message into a Text (using shared key).
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

// PackageRequest makes it easier to make a Message from Request.
// Uses the ZeroKey for pseudo-encryption of the message.
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

// PackageResponse makes it easier to make a Message from Response.
// Uses the ZeroKey for pseudo-encryption of the message.
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

// PackageText makes it easier to make a Message from Text.
// Requires a shared key for encrypting the Message.Payload.
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
