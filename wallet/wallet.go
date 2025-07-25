package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"log"

	"golang.org/x/crypto/ripemd160"
)

const (
	checksumLength = 4
	version        = byte(0x00)
)

type Wallet struct {
	PrivateKey []byte
	PublicKey  []byte
}

func (w Wallet) Address() []byte {
	pubHash := PublicKeyHash(w.PublicKey)

	versionedHash := append([]byte{version}, pubHash...)
	checkSum := Checksum(versionedHash)

	fullHash := append(versionedHash, checkSum...)
	address := Base58Encode(fullHash)

	fmt.Printf("pub key : %x\n", w.PublicKey)
	fmt.Printf("pub hash : %x\n", pubHash)
	fmt.Printf("address : %x\n", address)

	return address
}

func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}

	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)
	return *private, pub
}

func MakeWallet() *Wallet {
	private, public := NewKeyPair()

	privateBytes, err := x509.MarshalECPrivateKey(&private)
	if err != nil {
		log.Panic(err)
	}

	return &Wallet{
		PrivateKey: privateBytes,
		PublicKey:  public,
	}
}

func (w *Wallet) GetPrivateKey() *ecdsa.PrivateKey {
	privateKey, err := x509.ParseECPrivateKey(w.PrivateKey)
	if err != nil {
		log.Panic(err)
	}
	return privateKey
}

func PublicKeyHash(pubKey []byte) []byte {
	pubHash := sha256.Sum256(pubKey)

	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	if err != nil {
		log.Panic(err)
	}

	publicRipMD := hasher.Sum(nil)

	return publicRipMD
}

func Checksum(payload []byte) []byte {
	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLength]
}
