package tokens

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestIssueAndRedeemAnonymousToken(t *testing.T) {
	issuer, err := NewIssuer("k1", 1024, 2)
	if err != nil {
		t.Fatal(err)
	}
	pub := issuer.PublicKey()
	gateway := NewGateway(pub)
	gateway.now = func() time.Time { return time.Unix(100, 0) }

	clientToken, err := NewClientToken(pub.Key)
	if err != nil {
		t.Fatal(err)
	}

	blindSig, err := issuer.Issue("alice", clientToken.Blind)
	if err != nil {
		t.Fatal(err)
	}
	rawBlindSig, err := hex.DecodeString(blindSig.Signature)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := clientToken.Unblind(pub.Key, rawBlindSig)
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RedeemedAt != 100 {
		t.Fatalf("redeemed_at = %d, want 100", receipt.RedeemedAt)
	}
}

func TestQuotaIsEnforcedAtIssuer(t *testing.T) {
	issuer, err := NewIssuer("k1", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	pub := issuer.PublicKey()

	first, err := NewClientToken(pub.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Issue("alice", first.Blind); err != nil {
		t.Fatal(err)
	}

	second, err := NewClientToken(pub.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Issue("alice", second.Blind); !errors.Is(err, ErrNoQuota) {
		t.Fatalf("err = %v, want no quota", err)
	}
}

func TestRedeemRejectsDoubleSpend(t *testing.T) {
	issuer, err := NewIssuer("k1", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	pub := issuer.PublicKey()
	gateway := NewGateway(pub)

	clientToken, err := NewClientToken(pub.Key)
	if err != nil {
		t.Fatal(err)
	}
	blindSig, err := issuer.Issue("alice", clientToken.Blind)
	if err != nil {
		t.Fatal(err)
	}
	rawBlindSig, err := hex.DecodeString(blindSig.Signature)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := clientToken.Unblind(pub.Key, rawBlindSig)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig); !errors.Is(err, ErrAlreadySpent) {
		t.Fatalf("err = %v, want already spent", err)
	}
}

func TestGatewayCanAcceptRotatedKey(t *testing.T) {
	oldIssuer, err := NewIssuer("old", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	newIssuer, err := NewIssuer("new", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}

	gateway := NewGateway(oldIssuer.PublicKey())
	gateway.AddKey(newIssuer.PublicKey())

	clientToken, err := NewClientToken(newIssuer.PublicKey().Key)
	if err != nil {
		t.Fatal(err)
	}
	blindSig, err := newIssuer.Issue("alice", clientToken.Blind)
	if err != nil {
		t.Fatal(err)
	}
	rawBlindSig, err := hex.DecodeString(blindSig.Signature)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := clientToken.Unblind(newIssuer.PublicKey().Key, rawBlindSig)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkIssueAndRedeem(b *testing.B) {
	issuer, err := NewIssuer("bench", 2048, b.N+1)
	if err != nil {
		b.Fatal(err)
	}
	pub := issuer.PublicKey()
	gateway := NewGateway(pub)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		clientToken, err := NewClientToken(pub.Key)
		if err != nil {
			b.Fatal(err)
		}
		blindSig, err := issuer.Issue("benchmark-account", clientToken.Blind)
		if err != nil {
			b.Fatal(err)
		}
		rawBlindSig, err := hex.DecodeString(blindSig.Signature)
		if err != nil {
			b.Fatal(err)
		}
		sig, err := clientToken.Unblind(pub.Key, rawBlindSig)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig); err != nil {
			b.Fatal(err)
		}
	}
}
