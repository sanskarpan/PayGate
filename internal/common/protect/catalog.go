package protect

import "time"

type SecretClass string
type StorageBoundary string

const (
	ClassSigningSecret SecretClass = "signing_secret"
	ClassSessionSecret SecretClass = "session_secret"
	ClassEnvelopeKey   SecretClass = "envelope_key"
	ClassAPISecretHash SecretClass = "api_secret_hash_only"

	StorageBoundaryKMS      StorageBoundary = "kms_managed"
	StorageBoundaryDBCipher StorageBoundary = "encrypted_database_column"
	StorageBoundaryHashOnly StorageBoundary = "bcrypt_hash_only"
	StorageBoundaryRuntime  StorageBoundary = "runtime_secret_only"
)

type SecretSpec struct {
	Name            string
	Class           SecretClass
	StorageBoundary StorageBoundary
	RotationPeriod  time.Duration
	AccessBoundary  string
	ManagedBy       string
}

func CriticalSecretInventory() []SecretSpec {
	return []SecretSpec{
		{
			Name:            "dashboard_session_secret",
			Class:           ClassSessionSecret,
			StorageBoundary: StorageBoundaryRuntime,
			RotationPeriod:  90 * 24 * time.Hour,
			AccessBoundary:  "api-gateway dashboard auth only",
			ManagedBy:       "platform security",
		},
		{
			Name:            "payout_rail_callback_secret",
			Class:           ClassSigningSecret,
			StorageBoundary: StorageBoundaryRuntime,
			RotationPeriod:  90 * 24 * time.Hour,
			AccessBoundary:  "api-gateway payout callback verification only",
			ManagedBy:       "platform security",
		},
		{
			Name:            "app_encryption_root",
			Class:           ClassEnvelopeKey,
			StorageBoundary: StorageBoundaryKMS,
			RotationPeriod:  180 * 24 * time.Hour,
			AccessBoundary:  "app field encryption domains only",
			ManagedBy:       "platform security",
		},
		{
			Name:            "webhook_subscription_secret",
			Class:           ClassSigningSecret,
			StorageBoundary: StorageBoundaryDBCipher,
			RotationPeriod:  180 * 24 * time.Hour,
			AccessBoundary:  "webhook delivery pipeline and subscription management only",
			ManagedBy:       "merchant integration surface",
		},
		{
			Name:            "merchant_api_key_secret",
			Class:           ClassAPISecretHash,
			StorageBoundary: StorageBoundaryHashOnly,
			RotationPeriod:  180 * 24 * time.Hour,
			AccessBoundary:  "hash verification only; plaintext never recoverable",
			ManagedBy:       "merchant auth surface",
		},
	}
}
