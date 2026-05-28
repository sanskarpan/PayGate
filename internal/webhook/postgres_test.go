package webhook

import "testing"

func TestValidateSubscriptionURLRejectsPrivateHTTPSHosts(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://127.0.0.1/webhook",
		"https://10.0.0.5/webhook",
		"https://192.168.1.10/webhook",
		"https://172.16.0.10/webhook",
		"https://169.254.169.254/latest/meta-data",
		"https://100.64.1.2/hook",
	} {
		if err := validateSubscriptionURL(raw); err == nil {
			t.Fatalf("expected private https webhook URL %q to be rejected", raw)
		}
	}
}

func TestValidateSubscriptionURLAllowsOnlyLoopbackHTTP(t *testing.T) {
	t.Parallel()

	if err := validateSubscriptionURL("http://127.0.0.1:8080/hook"); err != nil {
		t.Fatalf("expected loopback http URL to be allowed, got %v", err)
	}
	if err := validateSubscriptionURL("http://localhost:8080/hook"); err != nil {
		t.Fatalf("expected localhost http URL to be allowed, got %v", err)
	}
	if err := validateSubscriptionURL("http://10.0.0.5/hook"); err == nil {
		t.Fatal("expected non-loopback http URL to be rejected")
	}
}
