// Command extkeygen makes the "key" an unpacked extension's manifest needs
// so that its ID never changes, and prints that ID.
//
//	go run ./cmd/extkeygen
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"

	"gale/internal/browsext"
)

func main() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded := base64.StdEncoding.EncodeToString(der)
	id, err := browsext.ExtensionID(encoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("key: %s\n", encoded)
	fmt.Printf("id:  %s\n", id)
}
