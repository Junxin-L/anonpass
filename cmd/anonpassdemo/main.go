package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"anonpass/internal/tokens"
)

func main() {
	issuer, err := tokens.NewIssuer("demo-1", 2048, 1)
	if err != nil {
		log.Fatal(err)
	}
	pub := issuer.PublicKey()
	gateway := tokens.NewGateway(pub)

	clientToken, err := tokens.NewClientToken(pub.Key)
	if err != nil {
		log.Fatal(err)
	}

	blindSig, err := issuer.Issue("alice@example.com", clientToken.Blind)
	if err != nil {
		log.Fatal(err)
	}
	rawBlindSig, err := hex.DecodeString(blindSig.Signature)
	if err != nil {
		log.Fatal(err)
	}

	sig, err := clientToken.Unblind(pub.Key, rawBlindSig)
	if err != nil {
		log.Fatal(err)
	}
	receipt, err := gateway.Redeem(blindSig.KeyID, clientToken.Token, sig)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("issued_to=alice@example.com")
	fmt.Printf("redeemed_token_hash=%s\n", receipt.TokenHash)
	fmt.Println("gateway_saw_account=false")
}
