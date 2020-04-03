package main

import (
	"bytes"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"time"
)

// Session stores information required for a specific "connection" between two
// chat clients, most important of which is likely the shared key for AES
// encryption.
type Session struct {
	Status      SessionStatus
	PrivKey     *rsa.PrivateKey
	SharedKey   []byte
	Other       *Profile
	OtherPubKey *rsa.PublicKey
	Expires     time.Time
}

// SessionIdleTimeout is the length of time a Session can go without
// receiving or sending (?) a Text from or to the other client. After timing out,
// a Session may be dropped and clients would need to initiate a new session.
const SessionIdleTimeout = 30 * time.Minute // TODO: make a sensible number

// SessionStatus is a session status.
type SessionStatus string

const (
	// Pending indicates the session is awaiting an a Response from another client.
	Pending SessionStatus = "pending"
	// Active indicates the session has negotiated a shared key and may send/receive Texts.
	Active SessionStatus = "active"
)

// InitiateSession creates a session based on intention to send Request to other.
func InitiateSession(me, other *Profile) (*Session, *Request, error) {
	if other == nil {
		return nil, nil, fmt.Errorf("nil Profile")
	}

	req, privKey, err := PrepareRequest(me)
	s := &Session{
		Status:  Pending,
		PrivKey: privKey,
		Other:   other,
		Expires: time.Now().Add(SessionIdleTimeout),
		// will not know SharedKey or OtherPubKey until received Response
	}

	return s, req, err
}

// BeginSession creates a session based on already having accepted a Request.
// It does much of the routine tasks involved in "accepting" a Request, including
// generating, encrypting, and signing a Shared Key, as well as creating a
// Response struct to be sent back to the other client.
func BeginSession(me *Profile, req *Request) (*Session, *Response, error) {
	// check that request isn't stale (older than session timeout)
	if time.Since(req.TimeStamp.Time()) > SessionIdleTimeout {
		return nil, nil, fmt.Errorf("request is stale")
	}

	respReq, privKey, err := PrepareRequest(me)
	if err != nil {
		return nil, nil, err
	}

	k, err := GenerateAES256Key()
	if err != nil {
		return nil, nil, err
	}
	encKey, err := RSAEncrypt(k, req.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	signature, err := SignRSA512(encKey, privKey)
	if err != nil {
		return nil, nil, err
	}

	resp := &Response{
		Request:      respReq,
		SharedKey:    encKey,
		KeySignature: signature,
	}

	s := &Session{
		Status:      Active,
		PrivKey:     privKey,
		SharedKey:   k,
		Other:       req.Profile,
		OtherPubKey: req.PublicKey,
		Expires:     time.Now().Add(SessionIdleTimeout),
	}

	return s, resp, err
}

// ExtendExpiration to now + `SessionIdleTimeout`.
func (s *Session) ExtendExpiration() {
	s.Expires = time.Now().Add(SessionIdleTimeout)
}

// IsExpired determines if a session is older than the max session timeout.
func (s *Session) IsExpired() bool { return time.Now().After(s.Expires) }

// String representation of the session.
func (s *Session) String() string {
	return fmt.Sprintf("[%s] %s\tleft: %s\tkey: %s",
		s.Status, s.Other,
		time.Until(s.Expires),
		base64.RawStdEncoding.EncodeToString(s.SharedKey))
}

// Equal compares sessions based on fields: Status, Expires, SharedKey, and Other.
func (s *Session) Equal(o *Session) bool {
	return o != nil &&
		bytes.Equal(s.SharedKey, o.SharedKey) && // probably most important thing
		s.Status == o.Status &&
		s.Expires.Equal(o.Expires) &&
		s.Other.Equal(o.Other)
}

// Upgrade attempts to use the Response to change a "pending" session into an
// "active" session. It does so by trying to decrypt the shared key in Response
// using the session's private RSA key. Any error results in a failure to upgrade
// and the session is not modified.
func (s *Session) Upgrade(resp *Response) error {
	if resp == nil {
		return fmt.Errorf("nil Response")
	}
	if s.Status != Pending {
		return fmt.Errorf("session is not Pending")
	}

	sharedKey, err := RSADecrypt(resp.SharedKey, s.PrivKey)
	if err != nil {
		return err
	}
	if !ValidSignatureRSA512(resp.KeySignature, resp.SharedKey, resp.Request.PublicKey) {
		return fmt.Errorf("invalid signature")
	}

	// shared key is now decrypted and the signature is valid
	// upgrade session
	s.Status = Active
	s.SharedKey = sharedKey
	s.OtherPubKey = resp.Request.PublicKey
	s.Other = resp.Request.Profile

	s.ExtendExpiration()
	return nil
}

// SendText does the routine work of sending a message string from one client
// to another. This includes packaging a Text into a Message and actually
// sending the Message on the network.
func (s *Session) SendText(message string) error {
	if s.Status != Active {
		return fmt.Errorf("session not Active")
	}
	if s.IsExpired() {
		return fmt.Errorf("session expired")
	}

	text := &Text{
		Message:   message,
		TimeStamp: Now(),
	}

	m, err := PackageText(text, s.SharedKey)
	if err != nil {
		return err
	}

	err = Send(s.Other.FullAddress(), m)
	if err != nil {
		return err
	}

	// TODO: perhaps don't want to extend when sending Text, only receiving?
	s.ExtendExpiration()
	return nil
}

// SendRequest does the routine work of sending chat request from one client
// to another. This includes packaging a Request into a Message and actually
// sending the Message on the network.
func (s *Session) SendRequest(req *Request) error {
	m, err := PackageRequest(req)
	if err != nil {
		return err
	}

	return Send(s.Other.FullAddress(), m)
}

// SendResponse does the routine work of sending a chat acceptance from one client
// to another. This includes packaging a Response into a Message and actually
// sending the Message on the network.
func (s *Session) SendResponse(resp *Response) error {

	m, err := PackageResponse(resp)
	if err != nil {
		return err
	}

	return Send(s.Other.FullAddress(), m)
}
