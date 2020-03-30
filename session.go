package main

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"time"
)

type Session struct {
	Status      SessionStatus
	PrivKey     *rsa.PrivateKey
	SharedKey   []byte
	Other       Profile
	OtherPubKey *rsa.PublicKey
	Expires     time.Time
}

const SessionIdleTimeout = 30 * time.Minute

type SessionStatus string

const (
	Pending SessionStatus = "pending"
	Active  SessionStatus = "active"
)

// creates a session based on intention to send Request to other.
func InitiateSession(me, other Profile) (*Session, Request, error) {
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

// creates a session based on already having accepted a Request.
func BeginSession(me Profile, req Request) (*Session, Response, error) {
	// check that request isn't stale (older than session timeout)
	if time.Since(req.TimeStamp.Time()) > SessionIdleTimeout {
		return nil, Response{}, fmt.Errorf("request is stale")
	}

	respReq, privKey, err := PrepareRequest(me)
	if err != nil {
		return nil, Response{}, err
	}

	k, err := GenerateAES256Key()
	if err != nil {
		return nil, Response{}, err
	}
	encKey, err := RSAEncrypt(k, &req.PublicKey)
	if err != nil {
		return nil, Response{}, err
	}
	signature, err := SignRSA512(encKey, privKey)
	if err != nil {
		return nil, Response{}, err
	}

	resp := Response{
		Request:      respReq,
		SharedKey:    encKey,
		KeySignature: signature,
	}

	s := &Session{
		Status:      Active,
		PrivKey:     privKey,
		SharedKey:   k,
		Other:       req.Profile,
		OtherPubKey: &req.PublicKey,
		Expires:     time.Now().Add(SessionIdleTimeout),
	}

	return s, resp, err
}

// ExtendExpiration to now + `SessionIdleTimeout`.
func (s *Session) ExtendExpiration() {
	s.Expires = time.Now().Add(SessionIdleTimeout)
}

func (s *Session) IsExpired() bool { return time.Now().After(s.Expires) }

func (s *Session) String() string {
	return fmt.Sprintf("[%s] %s\tleft: %s\tkey: %s",
		s.Status, s.Other,
		time.Until(s.Expires),
		base64.RawStdEncoding.EncodeToString(s.SharedKey))
}

func (s *Session) Upgrade(resp Response) error {
	if s.Status != Pending {
		return fmt.Errorf("session is not Pending")
	}

	sharedKey, err := RSADecrypt(resp.SharedKey, s.PrivKey)
	if err != nil {
		return err
	}
	if !ValidSignatureRSA512(resp.KeySignature, resp.SharedKey, &resp.Request.PublicKey) {
		return fmt.Errorf("invalid signature")
	}

	// shared key is now decrypted and the signature is valid
	// upgrade session
	s.Status = Active
	s.SharedKey = sharedKey
	s.OtherPubKey = &resp.Request.PublicKey
	s.Other = resp.Request.Profile

	s.ExtendExpiration()
	return nil
}

func (s *Session) SendText(message string) error {
	if s.Status != Active {
		return fmt.Errorf("session not Active")
	}
	if s.IsExpired() {
		return fmt.Errorf("session expired")
	}

	text := Text{
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

	s.ExtendExpiration()
	return nil
}

func (s *Session) SendRequest(req Request) error {
	m, err := PackageRequest(req)
	if err != nil {
		return err
	}

	return Send(s.Other.FullAddress(), m)
}

func (s *Session) SendResponse(resp Response) error {

	m, err := PackageResponse(resp)
	if err != nil {
		return err
	}

	return Send(s.Other.FullAddress(), m)
}
