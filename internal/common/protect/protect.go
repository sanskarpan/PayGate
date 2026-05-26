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

type Domain string

const (
	DomainGeneric                      Domain = "generic"
	DomainMerchantOnboardingIdentifier Domain = "merchant_onboarding_identifier"
	DomainMerchantOnboardingPartyPII   Domain = "merchant_onboarding_party_pii"
	DomainMerchantOnboardingDocument   Domain = "merchant_onboarding_document_storage"
	DomainReportingTaxProfile          Domain = "reporting_tax_profile"
	DomainPayoutBeneficiaryIdentity    Domain = "payout_beneficiary_identity"
	DomainWebhookSecret                Domain = "webhook_secret"
	DomainBillingCustomerPII           Domain = "billing_customer_pii"
	DomainBillingConnectedAccount      Domain = "billing_connected_account"
	DomainBillingInboundCollection     Domain = "billing_inbound_collection"
)

var (
	defaultProtector *Protector
	loadOnce         sync.Once
)

type Protector struct {
	activeVersion string
	keys          map[string][]byte
	enabled       bool
	provider      string
	kmsKeyURI     string
}

type Metadata struct {
	Provider      string
	ActiveVersion string
	Configured    []string
	Enabled       bool
	KMSKeyURI     string
}

func Default() *Protector {
	loadOnce.Do(func() {
		defaultProtector = mustFromEnv()
	})
	return defaultProtector
}

func mustFromEnv() *Protector {
	raw := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KEYS"))
	provider := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_PROVIDER"))
	kmsKeyURI := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KMS_KEY_URI"))
	if raw == "" {
		if provider == "" {
			provider = "disabled"
		}
		return &Protector{provider: provider, kmsKeyURI: kmsKeyURI}
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
	if provider == "" {
		provider = "env"
	}
	return &Protector{activeVersion: active, keys: keys, enabled: true, provider: provider, kmsKeyURI: kmsKeyURI}
}

func (p *Protector) SealString(value string) (string, error) {
	return p.SealStringForDomain(DomainGeneric, value)
}

func (p *Protector) SealStringForDomain(domain Domain, value string) (string, error) {
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
	aad := []byte(domain)
	ciphertext := gcm.Seal(nil, nonce, []byte(value), aad)
	payload := append(append([]byte{}, nonce...), ciphertext...)
	return envelopePrefix + p.activeVersion + ":" + string(domain) + ":" + base64.StdEncoding.EncodeToString(payload), nil
}

func (p *Protector) OpenString(value string) (string, error) {
	return p.OpenStringForDomain(DomainGeneric, value)
}

func (p *Protector) OpenStringForDomain(domain Domain, value string) (string, error) {
	if p == nil || !p.enabled || strings.TrimSpace(value) == "" || !strings.HasPrefix(value, envelopePrefix) {
		return value, nil
	}
	rest := strings.TrimPrefix(value, envelopePrefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 2 {
		return "", errors.New("invalid encrypted payload")
	}
	version := parts[0]
	payloadPart := ""
	cipherDomain := DomainGeneric
	switch len(parts) {
	case 2:
		payloadPart = parts[1]
	case 3:
		cipherDomain = Domain(parts[1])
		payloadPart = parts[2]
	default:
		return "", errors.New("invalid encrypted payload")
	}
	key, ok := p.keys[version]
	if !ok {
		return "", fmt.Errorf("missing encryption key version %s", version)
	}
	raw, err := base64.StdEncoding.DecodeString(payloadPart)
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
	aad := []byte(cipherDomain)
	if len(parts) == 2 {
		aad = nil
	} else if domain != DomainGeneric && cipherDomain != domain {
		return "", fmt.Errorf("ciphertext domain mismatch: expected %s got %s", domain, cipherDomain)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (p *Protector) Metadata() Metadata {
	if p == nil {
		return Metadata{Provider: "disabled"}
	}
	configured := make([]string, 0, len(p.keys))
	for version := range p.keys {
		configured = append(configured, version)
	}
	return Metadata{
		Provider:      p.provider,
		ActiveVersion: p.activeVersion,
		Configured:    configured,
		Enabled:       p.enabled,
		KMSKeyURI:     p.kmsKeyURI,
	}
}
