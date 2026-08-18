package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"anonpass/internal/tokens"
)

func TestRedeemRejectsMalformedHex(t *testing.T) {
	api, _ := newTestAPI(t)

	res := post(api, "/v1/gateway/redeem", map[string]string{
		"key_id":    "local",
		"token":     "not hex",
		"signature": "00",
	})

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request", res.Code)
	}
}

func TestConcurrentRedeemOnlyOneSucceeds(t *testing.T) {
	api, issuer := newTestAPI(t)
	pub := issuer.PublicKey()

	clientToken, err := tokens.NewClientToken(pub.Key)
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

	body := map[string]string{
		"key_id":    blindSig.KeyID,
		"token":     hex.EncodeToString(clientToken.Token),
		"signature": hex.EncodeToString(sig),
	}

	const attempts = 32
	var wg sync.WaitGroup
	statuses := make(chan int, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- post(api, "/v1/gateway/redeem", body).Code
		}()
	}
	wg.Wait()
	close(statuses)

	ok := 0
	conflict := 0
	for status := range statuses {
		switch status {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected status: %d", status)
		}
	}
	if ok != 1 || conflict != attempts-1 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
}

func TestDemoGatewayResponseDoesNotExposeAccount(t *testing.T) {
	api, _ := newTestAPI(t)

	issued := post(api, "/v1/demo/issue", map[string]string{
		"account": "alice@example.com",
	})
	if issued.Code != http.StatusOK {
		t.Fatalf("issue status = %d", issued.Code)
	}

	var session demoSession
	if err := json.Unmarshal(issued.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	redeemed := post(api, "/v1/demo/redeem", map[string]string{
		"session_id": session.ID,
	})
	if redeemed.Code != http.StatusOK {
		t.Fatalf("redeem status = %d", redeemed.Code)
	}
	if got := redeemed.Body.String(); strings.Contains(got, "alice@example.com") {
		t.Fatalf("gateway response exposed account: %s", got)
	}
}
