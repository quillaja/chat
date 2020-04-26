package main

import (
	"crypto/ed25519"
	"fmt"
)

// Message always uses AES256 and HMAC-SHA256.
// Generally use Package*() functions to create a new Message.
type Message struct {
	Payload   []byte      // chat request/response/text
	Signature []byte      // HMAC-SHA256 hash
	Type      PayloadType // used to process message into higher level types
	addr      string      // 'true' ip address where the message came from
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
	req, ok := gobDecode(m.Payload, m.Type).(*Request)
	if !ok {
		err = fmt.Errorf("message type wasn't Request")
		return
	}

	if !ValidSignatureEd25519(m.Signature, m.Payload, req.Profile.PublicSigningKey) {
		return nil, fmt.Errorf("invalid signature")
	}

	return
}

// GetResponse attempts to decrypt and decode the Message into a Response.
// SharedKey remains encrypted.
func (m *Message) GetResponse() (resp *Response, err error) {
	resp, ok := gobDecode(m.Payload, m.Type).(*Response)
	if !ok {
		err = fmt.Errorf("message type wasn't Response")
		return
	}

	if !ValidSignatureEd25519(m.Signature, m.Payload, resp.Profile.PublicSigningKey) {
		return nil, fmt.Errorf("invalid signature")
	}

	return
}

// GetText attempts to decrypt and decode the Message into a Text (using shared key).
func (m *Message) GetText(sharedKey []byte) (t *Text, err error) {
	plaintext, err := AESDecrypt(m.Payload, sharedKey)
	if err != nil {
		return
	}
	fmt.Printf("gettext plaintext %x\n", plaintext)

	if !ValidSignatureHS256(m.Signature, plaintext, sharedKey) {
		return nil, fmt.Errorf("invalid signature")
	}

	t, ok := gobDecode(plaintext, m.Type).(*Text)
	if !ok {
		err = fmt.Errorf("message type wasn't Text")
		return
	}

	return
}

// PackageRequest makes it easier to make a Message from Request.
func PackageRequest(req *Request, privSigningKey ed25519.PrivateKey) (m *Message, err error) {
	data, err := gobEncode(req)
	if err != nil {
		return
	}

	m = &Message{
		Payload:   data,
		Signature: SignEd25519(privSigningKey, data),
		Type:      PayloadRequest,
	}

	return
}

// PackageResponse makes it easier to make a Message from Response.
func PackageResponse(resp *Response, privSigningKey ed25519.PrivateKey) (m *Message, err error) {
	data, err := gobEncode(resp)
	if err != nil {
		return
	}

	m = &Message{
		Payload:   data,
		Signature: SignEd25519(privSigningKey, data),
		Type:      PayloadResponse,
	}

	return
}

// PackageText makes it easier to make a Message from Text.
//
// Encryption is done using AES256 in cipher block chaining (CBC) mode, and
// signing is done using HMAC-SHA256. Shared key should be 32 bytes to do
// AES256. Use GenerateAES256Key() to do so.
func PackageText(t *Text, sharedKey []byte) (m *Message, err error) {
	plaintext, err := gobEncode(t)
	if err != nil {
		return
	}

	// Use encrypt-and-mac scheme
	ciphertext, err := AESEncrypt(plaintext, sharedKey)
	if err != nil {
		return
	}

	// bodge to make sure the "plaintext" used in signing is padded (per AES)
	// so that the padded-then-decrypted plaintext on the other end will
	// pass signature validation
	plaintext, err = AESDecrypt(ciphertext, sharedKey)
	if err != nil {
		return
	}
	fmt.Printf("pkgtext plaintext %x\n", plaintext)

	m = &Message{
		Payload:   ciphertext,
		Signature: SignHS256(plaintext, sharedKey),
		Type:      PayloadText,
	}

	return
}
