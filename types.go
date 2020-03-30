package main

import (
	"crypto/rsa"
	"encoding/json"
	"io/ioutil"
	"time"
)

/*
Request is sent with key = []byte{}
Response is sent with key = []byte{}, but SharedKey/KeySignature is encrypted and signed by RSA public/private keys
Text is encrypted and signed by AES256 shared key
*/

var ZeroKey = make([]byte, 32)

type TimeStamp int64

func Now() TimeStamp                 { return TimeStamp(time.Now().Unix()) }
func (ts TimeStamp) Time() time.Time { return time.Unix(int64(ts), 0) }

type Profile struct {
	Name    string
	Address string // example "61.2.73.242" or "mytld.com"
	Port    string // port without :
}

type Request struct {
	Profile
	rsa.PublicKey
	TimeStamp // unix time in seconds
}

type Response struct {
	Request
	SharedKey    []byte // RSA encrypted proposed shared key (32 bytes) for this session
	KeySignature []byte // RSA signature for SharedKey
}

type Text struct {
	Message string // ideal max len 1024 bytes
	TimeStamp
}

func PrepareRequest(p Profile) (Request, *rsa.PrivateKey, error) {
	privateKey, err := GenerateRSAKeyPair()
	if err != nil {
		return Request{}, nil, err
	}

	r := Request{
		Profile:   p,
		TimeStamp: Now(),
		PublicKey: privateKey.PublicKey,
	}
	return r, privateKey, nil
}

func ReadProfile(filename string) (p Profile, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = json.Unmarshal(data, &p)
	return
}

func ReadContacts(filename string) (contacts []Profile, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &contacts)
	return
}

func WriteContacts(contacts []Profile, filename string) error {
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

func (p Profile) FullAddress() string { return p.Address + ":" + p.Port }

func (p Profile) String() string { return p.Name + "@" + p.FullAddress() }

func (p Profile) Equal(o Profile) bool { return p.FullAddress() == o.FullAddress() }
