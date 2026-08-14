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
)

type PublicKey struct {
	KeyID string         `json:"key_id"`
	Key   *rsa.PublicKey `json:"-"`
	N     string         `json:"n"`
	E     int            `json:"e"`
}

type BlindSignature struct {
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

type Receipt struct {
	TokenHash  string `json:"token_hash"`
	RedeemedAt int64  `json:"redeemed_at"`
}

type Issuer struct {
	mu      sync.Mutex
	keyID   string
	key     *rsa.PrivateKey
	quota   map[string]int
	perUser int
}

func NewIssuer(keyID string, bits int, perUser int) (*Issuer, error) {
	key, err := blindrsa.GenerateKey(bits)
	if err != nil {
		return nil, err
	}
	return &Issuer{
		keyID:   keyID,
		key:     key,
		quota:   make(map[string]int),
		perUser: perUser,
	}, nil
}

func (i *Issuer) PublicKey() PublicKey {
	i.mu.Lock()
	defer i.mu.Unlock()

	return publicKey(i.keyID, &i.key.PublicKey)
}

func (i *Issuer) Issue(account string, blindedToken []byte) (BlindSignature, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	left, ok := i.quota[account]
	if !ok {
		left = i.perUser
	}
	if left <= 0 {
		return BlindSignature{}, ErrNoQuota
	}

	sig, err := blindrsa.Sign(i.key, blindedToken)
	if err != nil {
		return BlindSignature{}, err
	}
	i.quota[account] = left - 1

	return BlindSignature{
		KeyID:     i.keyID,
		Signature: hex.EncodeToString(sig),
	}, nil
}

type Gateway struct {
	mu    sync.Mutex
	keys  map[string]*rsa.PublicKey
	spent map[string]Receipt
	now   func() time.Time
}

func NewGateway(keys ...PublicKey) *Gateway {
	g := &Gateway{
		keys:  make(map[string]*rsa.PublicKey),
		spent: make(map[string]Receipt),
		now:   time.Now,
	}
	for _, key := range keys {
		g.keys[key.KeyID] = key.Key
	}
	return g
}

func (g *Gateway) AddKey(key PublicKey) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.keys[key.KeyID] = key.Key
}

func (g *Gateway) Redeem(keyID string, token, signature []byte) (Receipt, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := g.keys[keyID]
	if key == nil {
		return Receipt{}, ErrUnknownKey
	}
	if !blindrsa.Verify(key, token, signature) {
		return Receipt{}, ErrBadToken
	}

	tokenHash := hashHex(token)
	if receipt, ok := g.spent[tokenHash]; ok {
		return receipt, ErrAlreadySpent
	}

	receipt := Receipt{
		TokenHash:  tokenHash,
		RedeemedAt: g.now().Unix(),
	}
	g.spent[tokenHash] = receipt
	return receipt, nil
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

func publicKey(keyID string, key *rsa.PublicKey) PublicKey {
	return PublicKey{
		KeyID: keyID,
		Key:   key,
		N:     key.N.Text(16),
		E:     key.E,
	}
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
