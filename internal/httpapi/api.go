package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"anonpass/internal/tokens"
)

type API struct {
	issuer  *tokens.Issuer
	gateway *tokens.Gateway
	mux     *http.ServeMux

	mu       sync.Mutex
	sessions map[string]demoSession
}

func New(issuer *tokens.Issuer, gateway *tokens.Gateway) *API {
	api := &API{
		issuer:   issuer,
		gateway:  gateway,
		mux:      http.NewServeMux(),
		sessions: make(map[string]demoSession),
	}
	api.routes()
	return api
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.mux.ServeHTTP(w, r)
}

func (api *API) routes() {
	api.mux.HandleFunc("GET /v1/issuer/key", api.getIssuerKey)
	api.mux.HandleFunc("POST /v1/issuer/blind-sign", api.issue)
	api.mux.HandleFunc("POST /v1/gateway/redeem", api.redeem)
	api.mux.HandleFunc("POST /v1/demo/issue", api.demoIssue)
	api.mux.HandleFunc("POST /v1/demo/redeem", api.demoRedeem)
}

func (api *API) getIssuerKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.issuer.PublicKey())
}

func (api *API) issue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account      string `json:"account"`
		BlindedToken string `json:"blinded_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json")
		return
	}
	if req.Account == "" || req.BlindedToken == "" {
		writeError(w, http.StatusBadRequest, "missing_field")
		return
	}

	blinded, err := hex.DecodeString(req.BlindedToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_blinded_token")
		return
	}

	sig, err := api.issuer.Issue(req.Account, blinded)
	if err != nil {
		if errors.Is(err, tokens.ErrNoQuota) {
			writeError(w, http.StatusTooManyRequests, "no_quota")
			return
		}
		writeError(w, http.StatusInternalServerError, "issue_failed")
		return
	}
	writeJSON(w, http.StatusOK, sig)
}

func (api *API) redeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyID     string `json:"key_id"`
		Token     string `json:"token"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json")
		return
	}

	token, err := hex.DecodeString(req.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_token")
		return
	}
	sig, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_signature")
		return
	}

	receipt, err := api.gateway.Redeem(req.KeyID, token, sig)
	if err != nil {
		switch {
		case errors.Is(err, tokens.ErrAlreadySpent):
			writeError(w, http.StatusConflict, "already_spent")
		case errors.Is(err, tokens.ErrUnknownKey):
			writeError(w, http.StatusBadRequest, "unknown_key")
		case errors.Is(err, tokens.ErrExpiredKey):
			writeError(w, http.StatusUnauthorized, "expired_key")
		case errors.Is(err, tokens.ErrBadToken):
			writeError(w, http.StatusUnauthorized, "bad_token")
		default:
			writeError(w, http.StatusInternalServerError, "redeem_failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (api *API) demoIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account string `json:"account"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json")
		return
	}
	if req.Account == "" {
		writeError(w, http.StatusBadRequest, "missing_account")
		return
	}

	pub := api.issuer.PublicKey()
	clientToken, err := tokens.NewClientToken(pub.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_token_failed")
		return
	}

	blindSig, err := api.issuer.Issue(req.Account, clientToken.Blind)
	if err != nil {
		if errors.Is(err, tokens.ErrNoQuota) {
			writeError(w, http.StatusTooManyRequests, "no_quota")
			return
		}
		writeError(w, http.StatusInternalServerError, "issue_failed")
		return
	}

	rawBlindSig, err := hex.DecodeString(blindSig.Signature)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bad_blind_signature")
		return
	}
	sig, err := clientToken.Unblind(pub.Key, rawBlindSig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unblind_failed")
		return
	}

	id, err := newSessionID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed")
		return
	}
	session := demoSession{
		ID:             id,
		Account:        req.Account,
		KeyID:          blindSig.KeyID,
		Remaining:      blindSig.Remaining,
		Token:          hex.EncodeToString(clientToken.Token),
		BlindedToken:   hex.EncodeToString(clientToken.Blind),
		BlindSignature: blindSig.Signature,
		Signature:      hex.EncodeToString(sig),
	}

	api.mu.Lock()
	api.sessions[id] = session
	api.mu.Unlock()

	writeJSON(w, http.StatusOK, session)
}

func (api *API) demoRedeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json")
		return
	}

	api.mu.Lock()
	session, ok := api.sessions[req.SessionID]
	api.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found")
		return
	}

	token, err := hex.DecodeString(session.Token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bad_session_token")
		return
	}
	sig, err := hex.DecodeString(session.Signature)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bad_session_signature")
		return
	}

	receipt, err := api.gateway.Redeem(session.KeyID, token, sig)
	if err != nil {
		writeJSON(w, httpStatusForRedeemError(err), map[string]any{
			"error":      errorCodeForRedeemError(err),
			"session_id": session.ID,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"receipt":    receipt,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

type demoSession struct {
	ID             string `json:"id"`
	Account        string `json:"account"`
	KeyID          string `json:"key_id"`
	Remaining      int    `json:"remaining"`
	Token          string `json:"token"`
	BlindedToken   string `json:"blinded_token"`
	BlindSignature string `json:"blind_signature"`
	Signature      string `json:"signature"`
}

func newSessionID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func httpStatusForRedeemError(err error) int {
	switch {
	case errors.Is(err, tokens.ErrAlreadySpent):
		return http.StatusConflict
	case errors.Is(err, tokens.ErrUnknownKey):
		return http.StatusBadRequest
	case errors.Is(err, tokens.ErrExpiredKey), errors.Is(err, tokens.ErrBadToken):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func errorCodeForRedeemError(err error) string {
	switch {
	case errors.Is(err, tokens.ErrAlreadySpent):
		return "already_spent"
	case errors.Is(err, tokens.ErrUnknownKey):
		return "unknown_key"
	case errors.Is(err, tokens.ErrExpiredKey):
		return "expired_key"
	case errors.Is(err, tokens.ErrBadToken):
		return "bad_token"
	default:
		return "redeem_failed"
	}
}
