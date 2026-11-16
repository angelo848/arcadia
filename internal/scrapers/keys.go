package scrapers

import (
	"errors"
	"fmt"
	"strings"
)

// ErrKeyNotFound is returned by a KeyStore for a name it does not hold.
var ErrKeyNotFound = errors.New("scrapers: chave não encontrada")

// KeyStore keeps the API keys of the providers outside the database (MID-001). The Windows
// implementation is the Credential Manager (winapi); tests use an in-memory one.
type KeyStore interface {
	Set(name, value string) error
	Get(name string) (string, error)
	Delete(name string) error
}

// Keys is the set of provider keys. A provider has one entry; IGDB keeps its client id and secret
// in that entry separated by a line break. No method returns a key in an error message (MID-001).
type Keys struct {
	store KeyStore
}

// NewKeys returns the keys held by st.
func NewKeys(st KeyStore) *Keys { return &Keys{store: st} }

// credName is the name of a provider's entry in the store.
func credName(provider string) string { return "arcadia/" + provider }

// Set stores the key of a provider. An empty key is refused and nothing is written (MID-001).
func (k *Keys) Set(provider, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return ErrNoKey
	}
	if err := k.store.Set(credName(provider), value); err != nil {
		return fmt.Errorf("scrapers: gravar chave de %s: %w", provider, err)
	}
	return nil
}

// Value returns the key of a provider, or ErrKeyNotFound.
func (k *Keys) Value(provider string) (string, error) {
	return k.store.Get(credName(provider))
}

// Configured reports whether a key is registered. The value itself is never shown (MID-001).
func (k *Keys) Configured(provider string) bool {
	_, err := k.store.Get(credName(provider))
	return err == nil
}

// Delete removes the key of a provider; a missing key is not an error.
func (k *Keys) Delete(provider string) error {
	if err := k.store.Delete(credName(provider)); err != nil && !errors.Is(err, ErrKeyNotFound) {
		return fmt.Errorf("scrapers: apagar chave de %s: %w", provider, err)
	}
	return nil
}

// SetIGDB stores the Twitch client id and secret of IGDB.
func (k *Keys) SetIGDB(clientID, secret string) error {
	clientID, secret = strings.TrimSpace(clientID), strings.TrimSpace(secret)
	if clientID == "" || secret == "" {
		return ErrNoKey
	}
	return k.Set("igdb", clientID+"\n"+secret)
}

// SetPair stores an id and a secret of a provider (ScreenScraper) as "id\nsecret". An empty half
// is refused with ErrNoKey and nothing is written (SSC-001).
func (k *Keys) SetPair(provider, id, secret string) error {
	if strings.ContainsAny(id, "\r\n") || strings.ContainsAny(secret, "\r\n") {
		return ErrInvalidKey
	}
	id, secret = strings.TrimSpace(id), strings.TrimSpace(secret)
	if id == "" || secret == "" {
		return ErrNoKey
	}
	return k.Set(provider, id+"\n"+secret)
}

// Pair returns the id and secret stored by SetPair; an entry without a line break is ErrKeyNotFound.
func (k *Keys) Pair(provider string) (id, secret string, err error) {
	v, err := k.Value(provider)
	if err != nil {
		return "", "", err
	}
	id, secret, ok := strings.Cut(v, "\n")
	if !ok {
		return "", "", ErrKeyNotFound
	}
	return id, secret, nil
}

// IGDB returns the client id and secret of IGDB.
func (k *Keys) IGDB() (clientID, secret string, err error) {
	v, err := k.Value("igdb")
	if err != nil {
		return "", "", err
	}
	clientID, secret, ok := strings.Cut(v, "\n")
	if !ok {
		return "", "", ErrKeyNotFound
	}
	return clientID, secret, nil
}
