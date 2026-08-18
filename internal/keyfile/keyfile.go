package keyfile

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"

	"anonpass/internal/blindrsa"
)

var ErrBadKeyFile = errors.New("bad key file")

func LoadOrCreate(path string, bits int) (*rsa.PrivateKey, error) {
	if path == "" {
		return blindrsa.GenerateKey(bits)
	}

	if data, err := os.ReadFile(path); err == nil {
		return parse(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key, err := blindrsa.GenerateKey(bits)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(pubPath(path), pem.EncodeToMemory(publicBlock(&key.PublicKey)), 0644); err != nil {
		return nil, err
	}
	return key, nil
}

func LoadPublic(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "RSA PUBLIC KEY" {
		return nil, ErrBadKeyFile
	}
	key, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func parse(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, ErrBadKeyFile
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, key.Validate()
}

func pubPath(path string) string {
	return path + ".pub"
}

func publicBlock(pub *rsa.PublicKey) *pem.Block {
	return &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(pub),
	}
}
