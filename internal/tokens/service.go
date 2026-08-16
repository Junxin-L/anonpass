package tokens

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"anonpass/internal/blindrsa"
)

var (
	ErrNoQuota      = errors.New("no quota left")
	ErrUnknownKey   = errors.New("unknown key")
	ErrBadToken     = errors.New("bad token")
	ErrAlreadySpent = errors.New("token already spent")
	ErrExpiredKey   = errors.New("key expired")
)

type PublicKey struct {
	KeyID    string         `json:"key_id"`
	Key      *rsa.PublicKey `json:"-"`
	N        string         `json:"n"`
	E        int            `json:"e"`
	NotAfter int64          `json:"not_after"`
}

type BlindSignature struct {
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
	Remaining int    `json:"remaining"`
}

type Receipt struct {
	TokenHash  string `json:"token_hash"`
	RedeemedAt int64  `json:"redeemed_at"`
}

type Issuer struct {
	mu       sync.Mutex
	keyID    string
	key      *rsa.PrivateKey
	notAfter int64
	quota    QuotaStore
	perUser  int
	now      func() time.Time
}

func NewIssuer(keyID string, bits int, perUser int) (*Issuer, error) {
	key, err := blindrsa.GenerateKey(bits)
	if err != nil {
		return nil, err
	}
	return NewIssuerWithKey(keyID, key, perUser, time.Now().Add(24*time.Hour).Unix()), nil
}

func NewIssuerWithKey(keyID string, key *rsa.PrivateKey, perUser int, notAfter int64) *Issuer {
	return NewIssuerWithStore(keyID, key, perUser, notAfter, NewMemoryQuotaStore())
}

func NewIssuerWithStore(keyID string, key *rsa.PrivateKey, perUser int, notAfter int64, quota QuotaStore) *Issuer {
	if quota == nil {
		quota = NewMemoryQuotaStore()
	}
	return &Issuer{
		keyID:    keyID,
		key:      key,
		notAfter: notAfter,
		quota:    quota,
		perUser:  perUser,
		now:      time.Now,
	}
}

func (i *Issuer) PublicKey() PublicKey {
	i.mu.Lock()
	defer i.mu.Unlock()

	return publicKey(i.keyID, &i.key.PublicKey, i.notAfter)
}

func (i *Issuer) Issue(account string, blindedToken []byte) (BlindSignature, error) {
	i.mu.Lock()
	keyID := i.keyID
	key := i.key
	perUser := i.perUser
	quota := i.quota
	window := i.window()
	i.mu.Unlock()

	remaining, ok, err := quota.Take(account, perUser, window)
	if err != nil {
		return BlindSignature{}, err
	}
	if !ok {
		return BlindSignature{}, ErrNoQuota
	}

	sig, err := blindrsa.Sign(key, blindedToken)
	if err != nil {
		return BlindSignature{}, err
	}

	return BlindSignature{
		KeyID:     keyID,
		Signature: hex.EncodeToString(sig),
		Remaining: remaining,
	}, nil
}

func (i *Issuer) window() string {
	return i.now().UTC().Format("2006-01-02")
}

type Gateway struct {
	mu    sync.Mutex
	keys  map[string]PublicKey
	store ReplayStore
	now   func() time.Time
}

func NewGateway(keys ...PublicKey) *Gateway {
	return NewGatewayWithStore(NewMemoryReplayStore(), keys...)
}

func NewGatewayWithStore(store ReplayStore, keys ...PublicKey) *Gateway {
	if store == nil {
		store = NewMemoryReplayStore()
	}
	g := &Gateway{
		keys:  make(map[string]PublicKey),
		store: store,
		now:   time.Now,
	}
	for _, key := range keys {
		g.keys[key.KeyID] = key
	}
	return g
}

func (g *Gateway) AddKey(key PublicKey) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.keys[key.KeyID] = key
}

func (g *Gateway) Redeem(keyID string, token, signature []byte) (Receipt, error) {
	g.mu.Lock()
	key, ok := g.keys[keyID]
	g.mu.Unlock()
	if !ok || key.Key == nil {
		return Receipt{}, ErrUnknownKey
	}
	if key.NotAfter > 0 && g.now().Unix() > key.NotAfter {
		return Receipt{}, ErrExpiredKey
	}
	if !blindrsa.Verify(key.Key, token, signature) {
		return Receipt{}, ErrBadToken
	}

	tokenHash := hashHex(token)
	receipt := Receipt{
		TokenHash:  tokenHash,
		RedeemedAt: g.now().Unix(),
	}

	out, inserted, err := g.store.Spend(tokenHash, receipt)
	if err != nil {
		return Receipt{}, err
	}
	if !inserted {
		return out, ErrAlreadySpent
	}
	return out, nil
}

type ClientToken struct {
	Token []byte
	Blind []byte
	state blindrsa.State
}

func NewClientToken(pub *rsa.PublicKey) (ClientToken, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return ClientToken{}, err
	}
	blinded, err := blindrsa.Blind(rand.Reader, pub, token)
	if err != nil {
		return ClientToken{}, err
	}
	return ClientToken{
		Token: blinded.Prepared,
		Blind: blinded.Value,
		state: blinded.State,
	}, nil
}

func (t ClientToken) Unblind(pub *rsa.PublicKey, blindSig []byte) ([]byte, error) {
	return blindrsa.Unblind(pub, blindSig, t.state)
}

func publicKey(keyID string, key *rsa.PublicKey, notAfter int64) PublicKey {
	return PublicKey{
		KeyID:    keyID,
		Key:      key,
		N:        key.N.Text(16),
		E:        key.E,
		NotAfter: notAfter,
	}
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
