package blindrsa

import "testing"

func FuzzVerifyRejectsRandomSignatures(f *testing.F) {
	key, err := GenerateKey(1024)
	if err != nil {
		f.Fatal(err)
	}

	f.Add([]byte("message"), []byte("signature"))
	f.Fuzz(func(t *testing.T, message, signature []byte) {
		if Verify(&key.PublicKey, message, signature) {
			t.Fatalf("random signature verified")
		}
	})
}
