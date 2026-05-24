package protect

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const envelopePrefix = "enc:"

var (
	defaultProtector *Protector
	loadOnce         sync.Once
)

type Protector struct {
	activeVersion string
	keys          map[string][]byte
	enabled       bool
}

func Default() *Protector {
	loadOnce.Do(func() {
		defaultProtector = mustFromEnv()
	})
	return defaultProtector
}

func mustFromEnv() *Protector {
	raw := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KEYS"))
	if raw == "" {
		return &Protector{}
	}
	keys := map[string][]byte{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			panic("APP_ENCRYPTION_KEYS entries must be version:base64key")
		}
		key, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			panic(fmt.Sprintf("invalid APP_ENCRYPTION_KEYS entry %s: %v", parts[0], err))
		}
		if len(key) != 32 {
			panic(fmt.Sprintf("APP_ENCRYPTION_KEYS key %s must be 32 bytes", parts[0]))
		}
		keys[parts[0]] = key
	}
	active := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_ACTIVE_KEY_VERSION"))
	if active == "" {
		for version := range keys {
			active = version
			break
		}
	}
	if _, ok := keys[active]; !ok {
		panic("APP_ENCRYPTION_ACTIVE_KEY_VERSION must reference a configured key")
	}
	return &Protector{activeVersion: active, keys: keys, enabled: true}
}

func (p *Protector) SealString(value string) (string, error) {
	if p == nil || !p.enabled || strings.TrimSpace(value) == "" {
		return value, nil
	}
	block, err := aes.NewCipher(p.keys[p.activeVersion])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	payload := append(nonce, ciphertext...)
	return envelopePrefix + p.activeVersion + ":" + base64.StdEncoding.EncodeToString(payload), nil
}

func (p *Protector) OpenString(value string) (string, error) {
	if p == nil || !p.enabled || strings.TrimSpace(value) == "" || !strings.HasPrefix(value, envelopePrefix) {
		return value, nil
	}
	rest := strings.TrimPrefix(value, envelopePrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid encrypted payload")
	}
	key, ok := p.keys[parts[0]]
	if !ok {
		return "", fmt.Errorf("missing encryption key version %s", parts[0])
	}
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted payload length")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
