package merchant

import "testing"

func TestMerchantValidateForCreate(t *testing.T) {
	cases := []struct {
		name    string
		m       Merchant
		wantErr bool
	}{
		{name: "valid", m: Merchant{Name: "ACME", Email: "a@b.com", BusinessType: "company"}, wantErr: false},
		{name: "missing name", m: Merchant{Email: "a@b.com", BusinessType: "company"}, wantErr: true},
		{name: "missing email", m: Merchant{Name: "ACME", BusinessType: "company"}, wantErr: true},
		{name: "missing business type", m: Merchant{Name: "ACME", Email: "a@b.com"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.ValidateForCreate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestScopeAllows(t *testing.T) {
	if !ScopeAllows(APIKeyScopeAdmin, APIKeyScopeWrite) {
		t.Fatal("admin should allow write")
	}
	if ScopeAllows(APIKeyScopeRead, APIKeyScopeWrite) {
		t.Fatal("read should not allow write")
	}
	if !ScopeAllows(APIKeyScopeWrite, APIKeyScopeRead) {
		t.Fatal("write should allow read")
	}
}
