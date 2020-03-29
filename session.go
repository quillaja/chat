package main

import (
	"crypto/rsa"
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

type SessionStatus byte

const (
	Pending SessionStatus = iota
	Active
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
	// TODO: check that request isn't stale (older than session timeout)
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

func (s *Session) ID() string { return s.Other.FullAddress() }

func (s *Session) SendText(message string) error {
	text := Text{
		Message:   message,
		TimeStamp: Now(),
	}

	m, err := PackageText(text, s.SharedKey)
	if err != nil {
		return err
	}

	return Send(s.Other.FullAddress(), m)
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
