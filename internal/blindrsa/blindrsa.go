package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"

	circlrsa "github.com/cloudflare/circl/blindsign/blindrsa"
)

var ErrBadKey = errors.New("bad rsa key")

type BlindedMessage struct {
	Prepared []byte
	Value    []byte
	State    circlrsa.State
}

type State = circlrsa.State

func GenerateKey(bits int) (*rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	return key, key.Validate()
}

func Blind(random io.Reader, pub *rsa.PublicKey, message []byte) (BlindedMessage, error) {
	if pub == nil || pub.N == nil || pub.E < 3 {
		return BlindedMessage{}, ErrBadKey
	}

	client, err := circlrsa.NewClient(circlrsa.SHA384PSSRandomized, pub)
	if err != nil {
		return BlindedMessage{}, err
	}
	prepared, err := client.Prepare(random, message)
	if err != nil {
		return BlindedMessage{}, err
	}
	blinded, state, err := client.Blind(random, prepared)
	if err != nil {
		return BlindedMessage{}, err
	}

	return BlindedMessage{
		Prepared: prepared,
		Value:    blinded,
		State:    state,
	}, nil
}

func Sign(priv *rsa.PrivateKey, blinded []byte) ([]byte, error) {
	if priv == nil || priv.N == nil || priv.D == nil {
		return nil, ErrBadKey
	}
	if err := priv.Validate(); err != nil {
		return nil, err
	}

	signer := circlrsa.NewSigner(priv)
	return signer.BlindSign(blinded)
}

func Unblind(pub *rsa.PublicKey, blindedSig []byte, state circlrsa.State) ([]byte, error) {
	if pub == nil || pub.N == nil || pub.E < 3 {
		return nil, ErrBadKey
	}

	client, err := circlrsa.NewClient(circlrsa.SHA384PSSRandomized, pub)
	if err != nil {
		return nil, err
	}
	return client.Finalize(state, blindedSig)
}

func Verify(pub *rsa.PublicKey, prepared []byte, signature []byte) bool {
	if pub == nil || pub.N == nil || pub.E < 3 {
		return false
	}

	verifier, err := circlrsa.NewVerifier(circlrsa.SHA384PSSRandomized, pub)
	if err != nil {
		return false
	}
	return verifier.Verify(prepared, signature) == nil
}
