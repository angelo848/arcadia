package agent

import (
	"errors"
	"net/http"
	"slices"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// errUnknownProvider is returned for a provider name the contract does not list.
var errUnknownProvider = errors.New("agent: provedor desconhecido")

// providerNames are the providers of contract v0.9.0; the keys are for the first two only.
var providerNames = []string{"igdb", "steamgriddb", "steamstore"}

// keyProviderNames are the providers that need a key (MID-001).
var keyProviderNames = []string{"igdb", "steamgriddb"}

func validProvider(name string) bool { return slices.Contains(providerNames, name) }

func validKeyProvider(name string) bool { return slices.Contains(keyProviderNames, name) }

// credentialStore adapts the Windows Credential Manager to scrapers.KeyStore: a missing entry is
// scrapers.ErrKeyNotFound, so the scrapers package does not depend on Windows (MID-001).
type credentialStore struct {
	st winapi.CredentialStore
}

func (c credentialStore) Set(name, value string) error { return c.st.Set(name, value) }

func (c credentialStore) Get(name string) (string, error) {
	v, err := c.st.Get(name)
	if errors.Is(err, winapi.ErrCredentialNotFound) {
		return "", scrapers.ErrKeyNotFound
	}
	return v, err
}

func (c credentialStore) Delete(name string) error { return c.st.Delete(name) }

// newProviderFactory returns the function that builds a provider with the keys registered at the
// moment of the call, so a key saved in the UI is used at once (MID-001).
func newProviderFactory(keys *scrapers.Keys, client *http.Client, clock domain.Clock) func(name string) (scrapers.MetadataProvider, error) {
	return func(name string) (scrapers.MetadataProvider, error) {
		switch name {
		case "igdb":
			id, secret, err := keys.IGDB()
			if err != nil {
				return nil, keyError(err)
			}
			return scrapers.NewIGDB(scrapers.IGDBConfig{ClientID: id, Secret: secret, HTTP: client, Clock: clock}), nil
		case "steamgriddb":
			key, err := keys.Value("steamgriddb")
			if err != nil {
				return nil, keyError(err)
			}
			return scrapers.NewSteamGridDB(scrapers.SGDBConfig{Key: key, HTTP: client}), nil
		case "steamstore":
			return scrapers.NewSteamStore(scrapers.SteamConfig{HTTP: client}), nil
		default:
			return nil, errUnknownProvider
		}
	}
}

// keyError makes a missing key the same error as a key the provider refuses to use (MID-001).
func keyError(err error) error {
	if errors.Is(err, scrapers.ErrKeyNotFound) {
		return scrapers.ErrNoKey
	}
	return err
}
