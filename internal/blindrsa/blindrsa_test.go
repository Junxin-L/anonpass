package blindrsa

import (
	"crypto/rand"
	"testing"
)

func TestBlindSignUnblindVerify(t *testing.T) {
	key, err := GenerateKey(1024)
	if err != nil {
		t.Fatal(err)
	}

	token := []byte("token-32-random-bytes-would-live-here")
	blinded, err := Blind(rand.Reader, &key.PublicKey, token)
	if err != nil {
		t.Fatal(err)
	}

	blindSig, err := Sign(key, blinded.Value)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := Unblind(&key.PublicKey, blindSig, blinded.State)
	if err != nil {
		t.Fatal(err)
	}

	if !Verify(&key.PublicKey, blinded.Prepared, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestVerifyRejectsDifferentToken(t *testing.T) {
	key, err := GenerateKey(1024)
	if err != nil {
		t.Fatal(err)
	}

	blinded, err := Blind(rand.Reader, &key.PublicKey, []byte("alice-token"))
	if err != nil {
		t.Fatal(err)
	}
	blindSig, err := Sign(key, blinded.Value)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := Unblind(&key.PublicKey, blindSig, blinded.State)
	if err != nil {
		t.Fatal(err)
	}

	if Verify(&key.PublicKey, []byte("bob-token"), sig) {
		t.Fatal("signature verified for the wrong token")
	}
}

func BenchmarkBlindSignUnblindVerify(b *testing.B) {
	key, err := GenerateKey(2048)
	if err != nil {
		b.Fatal(err)
	}
	token := []byte("token-32-random-bytes-would-live-here")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		blinded, err := Blind(rand.Reader, &key.PublicKey, token)
		if err != nil {
			b.Fatal(err)
		}
		blindSig, err := Sign(key, blinded.Value)
		if err != nil {
			b.Fatal(err)
		}
		sig, err := Unblind(&key.PublicKey, blindSig, blinded.State)
		if err != nil {
			b.Fatal(err)
		}
		if !Verify(&key.PublicKey, blinded.Prepared, sig) {
			b.Fatal("valid signature rejected")
		}
	}
}
