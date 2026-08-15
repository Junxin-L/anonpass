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
	return key, os.WriteFile(path, pem.EncodeToMemory(block), 0600)
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
