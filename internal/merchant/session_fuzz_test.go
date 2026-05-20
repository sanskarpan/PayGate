package merchant

import "testing"

func FuzzSessionManagerParse(f *testing.F) {
	manager := NewSessionManager("0123456789abcdef0123456789abcdef", 0)
	if manager == nil {
		f.Fatal("expected session manager")
	}

	seedUser := MerchantUser{
		ID:         "muser_seed",
		MerchantID: "merch_seed",
		Email:      "seed@example.com",
		Role:       MerchantUserRoleAdmin,
	}
	token, err := manager.Issue(seedUser)
	if err != nil {
		f.Fatalf("issue seed token: %v", err)
	}

	f.Add(token)
	f.Add("")
	f.Add("not-a-token")
	f.Add("abc.def.ghi")

	f.Fuzz(func(t *testing.T, raw string) {
		claims, err := manager.Parse(raw)
		if err == nil {
			if claims.UserID == "" || claims.MerchantID == "" || claims.Email == "" {
				t.Fatalf("successful parse returned incomplete claims: %+v", claims)
			}
		}
	})
}
