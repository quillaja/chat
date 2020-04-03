package main

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
)

/*

AES

*/

// GenerateAES256Key makes a 32 byte key using a cryptographically acceptable
// source of random data.
func GenerateAES256Key() ([]byte, error) {
	const size = 32 // 32 bytes for AES256
	key := make([]byte, size)
	_, err := rand.Read(key)
	return key, err
}

// AESDecrypt ciphertext to plaintext using key.
// Key length of 16 bytes (AES128) or 32 bytes (AES256)
func AESDecrypt(ciphertext, key []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}

	if len(ciphertext) < aes.BlockSize {
		err = fmt.Errorf("ciphertext too short")
		return
	}
	iv := ciphertext[:aes.BlockSize]        // trim (unencrypted) iv off ciphertext
	ciphertext = ciphertext[aes.BlockSize:] // isolate true ciphertext

	if len(ciphertext)%aes.BlockSize != 0 {
		err = fmt.Errorf("ciphertext is not a multiple of the block size")
		return
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	plaintext = make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)
	return
}

// AESEncrypt plaintext to ciphertext using key.
func AESEncrypt(plaintext, key []byte) (ciphertext []byte, err error) {
	// plaintext should be a multiple of the block size.
	// here plaintext is padded with 0, but
	// https://tools.ietf.org/html/rfc5246#section-6.2.3.2
	// suggests padding with the padding size. meh.
	padding := (aes.BlockSize - (len(plaintext) % aes.BlockSize)) % aes.BlockSize
	plaintext = append(plaintext, make([]byte, padding)...)

	if len(plaintext)%aes.BlockSize != 0 {
		err = fmt.Errorf("ciphertext is not a multiple of the block size")
		return
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}

	// prepend iv to ciphertext
	ciphertext = make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	_, err = rand.Read(iv) // fill iv with random bytes
	if err != nil {
		return
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], plaintext) // encrypt plaintext but not iv
	return
}

// SignHS256 creates a signature of message using the HMAC-SHA256 hash.
func SignHS256(message, key []byte) (signature []byte) {
	mac := hmac.New(sha256.New, key)
	mac.Write(message) // signature is a hash of payload
	signature = mac.Sum(nil)
	return
}

// ValidSignatureHS256 validates that a signaure matches the expected signature.
func ValidSignatureHS256(signature, message, key []byte) bool {
	expectedMAC := SignHS256(message, key)
	return hmac.Equal(signature, expectedMAC)
}

// SignHS512 creates a signature of message using the HMAC-SHA512 hash.
func SignHS512(message, key []byte) (signature []byte) {
	mac := hmac.New(sha512.New, key)
	mac.Write(message) // signature is a hash of payload
	signature = mac.Sum(nil)
	return
}

// ValidSignatureHS512 validates that a signaure matches the expected signature.
func ValidSignatureHS512(signature, message, key []byte) bool {
	expectedMAC := SignHS512(message, key)
	return hmac.Equal(signature, expectedMAC)
}

/*

RSA

*/

// GenerateRSAKeyPair creates a public/private key pair using a 2048 bit RSA key.
func GenerateRSAKeyPair() (*rsa.PrivateKey, error) {
	// use 2048 bits here to keep rsa keys from being huge.
	// 4096 bits for better security (vs 2048 and smaller)
	const size = 2048
	return rsa.GenerateKey(rand.Reader, size)
}

// RSAEncrypt plaintext to ciphertext using RSA-OAEP with a SHA-512 hash.
func RSAEncrypt(plaintext []byte, receiverKey *rsa.PublicKey) (ciphertext []byte, err error) {
	ciphertext, err = rsa.EncryptOAEP(sha512.New(), rand.Reader, receiverKey, plaintext, []byte{})
	return
}

// RSADecrypt ciphertext to plaintext using RSA-OAEP with a SHA-512 hash.
func RSADecrypt(ciphertext []byte, receiverKey *rsa.PrivateKey) (plaintext []byte, err error) {
	plaintext, err = rsa.DecryptOAEP(sha512.New(), rand.Reader, receiverKey, ciphertext, []byte{})
	return
}

// SignRSA512 signs message using key and SHA-512.
func SignRSA512(message []byte, senderKey *rsa.PrivateKey) (signature []byte, err error) {
	hashed := sha512.Sum512(message) // 64 bytes long
	signature, err = rsa.SignPSS(rand.Reader, senderKey, crypto.SHA512, hashed[:], nil)
	return
}

// ValidSignatureRSA512 validates that signature matches the expected signature.
func ValidSignatureRSA512(signature, message []byte, senderKey *rsa.PublicKey) (valid bool) {
	hashed := sha512.Sum512(message)
	err := rsa.VerifyPSS(senderKey, crypto.SHA512, hashed[:], signature, nil)
	valid = err == nil
	if err != nil {
		fmt.Println(err) // debug only. non-nil err indicates invalid message/signature
	}
	return
}

// SignRSA256 signs message using key and SHA-256.
func SignRSA256(message []byte, senderKey *rsa.PrivateKey) (signature []byte, err error) {
	hashed := sha256.Sum256(message) // 64 bytes long
	signature, err = rsa.SignPSS(rand.Reader, senderKey, crypto.SHA256, hashed[:], nil)
	return
}

// ValidSignatureRSA256 validates that signature matches the expected signature.
func ValidSignatureRSA256(signature, message []byte, senderKey *rsa.PublicKey) (valid bool) {
	hashed := sha256.Sum256(message)
	err := rsa.VerifyPSS(senderKey, crypto.SHA256, hashed[:], signature, nil)
	valid = err == nil
	if err != nil {
		fmt.Println(err) // debug only. non-nil err indicates invalid message/signature
	}
	return
}
