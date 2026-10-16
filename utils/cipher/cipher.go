// Package cipher implements the AES-256-CBC/PBKDF2 scheme produced by
// `openssl enc -aes-256-cbc -pbkdf2 -iter 10000 -salt`.
package cipher

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltLen    = 8
	keyLen     = 32
	ivLen      = 16
	iterations = 10000
)

var saltedMagic = []byte("Salted__")

// Public functions

// Encrypt matches `openssl enc -aes-256-cbc -pbkdf2 -iter 10000 -salt
// -pass "pass:<password>" | base64 -w0` byte for byte.
func Encrypt(plaintext []byte, password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("cipher: generating salt: %w", err)
	}

	key, iv := deriveKeyIV(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher: %w", err)
	}

	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	out := make([]byte, 0, len(saltedMagic)+saltLen+len(ciphertext))
	out = append(out, saltedMagic...)
	out = append(out, salt...)
	out = append(out, ciphertext...)

	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt, matching `base64 -d | openssl enc -d
// -aes-256-cbc -pbkdf2 -iter 10000 -pass "pass:<password>"`.
func Decrypt(encoded string, password string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("cipher: invalid base64: %w", err)
	}
	if len(raw) < len(saltedMagic)+saltLen || !bytes.Equal(raw[:len(saltedMagic)], saltedMagic) {
		return nil, errors.New("cipher: missing Salted__ header")
	}

	salt := raw[len(saltedMagic) : len(saltedMagic)+saltLen]
	ciphertext := raw[len(saltedMagic)+saltLen:]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("cipher: invalid ciphertext length")
	}

	key, iv := deriveKeyIV(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}

	padded := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ciphertext)

	plaintext, err := pkcs7Unpad(padded, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("cipher: could not decrypt — wrong key or corrupted data: %w", err)
	}
	return plaintext, nil
}

// Helpers

// deriveKeyIV runs PBKDF2-HMAC-SHA256 once and splits its output into the
// AES key and IV, matching OpenSSL's -pbkdf2 key derivation.
func deriveKeyIV(password string, salt []byte) (key, iv []byte) {
	derived := pbkdf2.Key([]byte(password), salt, iterations, keyLen+ivLen, sha256.New)
	return derived[:keyLen], derived[keyLen:]
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("cipher: invalid padded data length")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("cipher: invalid padding")
	}
	// Every padding byte must equal padLen, not just the last one.
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, errors.New("cipher: invalid padding")
		}
	}
	return data[:len(data)-padLen], nil
}
