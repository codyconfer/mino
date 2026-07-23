package token

import (
	"encoding/base64"
	"encoding/json"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/sisyphus/backup"
	"github.com/codyconfer/sisyphus/kv"
	"github.com/codyconfer/sisyphus/secret"
)

const (
	namespace      = "tokens"
	keyringService = "munin"
	keyName        = "munin-token-key"
)

var errUnavailable = errs.New(errs.KindStore, "token store unavailable")

func keyringKey() ([]byte, error) {
	store, err := secret.Resolve("keyring", keyringService)
	if err != nil {
		return nil, err
	}
	v, err := secret.GetOrCreate(store, keyName, func() (string, error) {
		k, err := backup.NewKey()
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(k), nil
	})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(v)
}

type Store struct {
	kv          *kv.Store
	keyProvider func() ([]byte, error)
	key         []byte
}

func Open(path string) (*Store, error) {
	k, err := kv.Open(path)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open token store")
	}
	return &Store{kv: k, keyProvider: keyringKey}, nil
}

func (s *Store) Close() error {
	if s == nil || s.kv == nil {
		return nil
	}
	if err := s.kv.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close token store")
	}
	return nil
}

func (s *Store) encryptionKey() ([]byte, error) {
	if s.key != nil {
		return s.key, nil
	}
	provider := s.keyProvider
	if provider == nil {
		provider = keyringKey
	}
	k, err := provider()
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "acquire token key")
	}
	s.key = k
	return k, nil
}

func (s *Store) Get(service string) (auth.Credential, bool, error) {
	if s == nil || s.kv == nil {
		return auth.Credential{}, false, nil
	}
	e, ok, err := s.kv.Get(namespace, service)
	if err != nil {
		return auth.Credential{}, ok, errs.Wrap(errs.KindStore, err, "read token")
	}
	if !ok {
		return auth.Credential{}, false, nil
	}
	key, err := s.encryptionKey()
	if err != nil {
		return auth.Credential{}, false, err
	}
	var c auth.Credential
	if sealed, derr := base64.StdEncoding.DecodeString(e.Value); derr == nil {
		if plain, derr := backup.Decrypt(sealed, key); derr == nil {
			if json.Unmarshal(plain, &c) == nil {
				return c, true, nil
			}
		}
	}
	if json.Unmarshal([]byte(e.Value), &c) == nil {
		return c, true, nil
	}
	return auth.Credential{}, false, nil
}

func (s *Store) Put(service string, c auth.Credential) error {
	if s == nil || s.kv == nil {
		return errUnavailable
	}
	key, err := s.encryptionKey()
	if err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := backup.Encrypt(b, key)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "encrypt token")
	}
	v := base64.StdEncoding.EncodeToString(sealed)
	if err := s.kv.Put(namespace, service, v, c.Expiry); err != nil {
		return errs.Wrap(errs.KindStore, err, "write token")
	}
	return nil
}

func (s *Store) Delete(service string) error {
	if s == nil || s.kv == nil {
		return errUnavailable
	}
	if err := s.kv.Delete(namespace, service); err != nil {
		return errs.Wrap(errs.KindStore, err, "delete token")
	}
	return nil
}
