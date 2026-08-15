package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"anonpass/internal/tokens"
)

type API struct {
	issuer  *tokens.Issuer
	gateway *tokens.Gateway
	mux     *http.ServeMux
}

func New(issuer *tokens.Issuer, gateway *tokens.Gateway) *API {
	api := &API{
		issuer:  issuer,
		gateway: gateway,
		mux:     http.NewServeMux(),
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
		writeError(w, http.StatusBadRequest, "issue_failed")
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
