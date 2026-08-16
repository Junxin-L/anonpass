package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"anonpass/internal/blindrsa"
	"anonpass/internal/tokens"
)

func TestIssueEndpoint(t *testing.T) {
	api, issuer := newTestAPI(t)
	pub := issuer.PublicKey()

	blinded, err := blindrsa.Blind(rand.Reader, pub.Key, []byte("client-token"))
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]string{
		"account":       "alice",
		"blinded_token": hex.EncodeToString(blinded.Value),
	}
	res := post(api, "/v1/issuer/blind-sign", body)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestRedeemEndpointRejectsReplay(t *testing.T) {
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
	first := post(api, "/v1/gateway/redeem", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}

	second := post(api, "/v1/gateway/redeem", body)
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want conflict", second.Code)
	}
}

func TestDemoIssueRedeemReplay(t *testing.T) {
	api, _ := newTestAPI(t)

	issued := post(api, "/v1/demo/issue", map[string]string{
		"account": "alice@example.com",
	})
	if issued.Code != http.StatusOK {
		t.Fatalf("issue status = %d, body = %s", issued.Code, issued.Body.String())
	}

	var session demoSession
	if err := json.Unmarshal(issued.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.Token == "" || session.Signature == "" {
		t.Fatalf("incomplete session: %+v", session)
	}

	redeemed := post(api, "/v1/demo/redeem", map[string]string{
		"session_id": session.ID,
	})
	if redeemed.Code != http.StatusOK {
		t.Fatalf("redeem status = %d, body = %s", redeemed.Code, redeemed.Body.String())
	}

	replayed := post(api, "/v1/demo/redeem", map[string]string{
		"session_id": session.ID,
	})
	if replayed.Code != http.StatusConflict {
		t.Fatalf("replay status = %d, want conflict", replayed.Code)
	}
}

func newTestAPI(t *testing.T) (http.Handler, *tokens.Issuer) {
	t.Helper()
	issuer, err := tokens.NewIssuer("local", 1024, 2)
	if err != nil {
		t.Fatal(err)
	}
	return New(issuer, tokens.NewGateway(issuer.PublicKey())), issuer
}

func post(handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
