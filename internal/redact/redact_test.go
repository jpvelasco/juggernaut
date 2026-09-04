package redact

import (
	"strings"
	"testing"
)

func TestString_RedactsTokenAccountAndHome(t *testing.T) {
	home := `C:\Users\alice\AppData\Local\Temp\juggernaut-test`
	in := strings.Join([]string{
		"Authorization: Bearer super-secret-token-value",
		"account 123456789012",
		"path " + home + `\settings.json`,
		"also /Users/bob/.claude/settings.json",
	}, "\n")

	out := String(in, Options{Home: home})
	for _, secret := range []string{
		"super-secret-token-value",
		"123456789012",
		home,
		`\Users\alice`,
		"/Users/bob",
	} {
		if strings.Contains(out, secret) {
			t.Errorf("redacted output still contains %q:\n%s", secret, out)
		}
	}
	for _, placeholder := range []string{Token, AccountID, Home} {
		if !strings.Contains(out, placeholder) {
			t.Errorf("expected placeholder %s in:\n%s", placeholder, out)
		}
		if placeholder == "" {
			t.Fatal("placeholder must not be empty")
		}
	}
}

func TestString_RedactsAccessKeyARNEmailHostnameAndLANIP(t *testing.T) {
	accessKey := "AKIA" + "TESTKEYNOTREAL00"
	in := strings.Join([]string{
		"key " + accessKey,
		"arn:aws:iam::123456789012:role/Admin",
		"user  test.user@example.com",
		"host my-laptop.local",
		"lan 192.168.1.50",
		"loop 127.0.0.1",
	}, "\n")

	out := String(in, Options{Hostname: "my-laptop.local"})
	if strings.Contains(out, accessKey) {
		t.Errorf("access key still present:\n%s", out)
	}
	if strings.Contains(out, "arn:aws:iam::") {
		t.Errorf("ARN still present:\n%s", out)
	}
	if strings.Contains(out, "test.user@example.com") {
		t.Errorf("email still present:\n%s", out)
	}
	if strings.Contains(out, "my-laptop.local") {
		t.Errorf("hostname still present:\n%s", out)
	}
	if strings.Contains(out, "192.168.1.50") || strings.Contains(out, "127.0.0.1") {
		t.Errorf("LAN/loopback IP still present:\n%s", out)
	}
	if !strings.Contains(out, AccessKeyID) || !strings.Contains(out, ARN) || !strings.Contains(out, Email) {
		t.Errorf("missing placeholders:\n%s", out)
	}
	if !strings.Contains(out, Hostname) || !strings.Contains(out, IP) {
		t.Errorf("missing host/ip placeholders:\n%s", out)
	}
}

func TestString_ReplacesExplicitSecretsFirst(t *testing.T) {
	out := String("token=abc-literal-secret", Options{Secrets: []string{"abc-literal-secret"}})
	if strings.Contains(out, "abc-literal-secret") {
		t.Fatalf("secret still present: %s", out)
	}
	if !strings.Contains(out, Token) {
		t.Fatalf("expected %s, got %s", Token, out)
	}
}

func TestString_EmptyInputUnchanged(t *testing.T) {
	if got := String("", Options{Home: "/tmp"}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestString_DoesNotRedactPublicIP(t *testing.T) {
	in := "reach 8.8.8.8"
	out := String(in, Options{})
	if !strings.Contains(out, "8.8.8.8") {
		t.Fatalf("public IP should remain, got %s", out)
	}
}

func TestString_BedrockAPIKeyAndJWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiIxIn0." + "abc"
	in := "bedrock-api-key-AAAA\n" + jwt
	out := String(in, Options{})
	if strings.Contains(out, "bedrock-api-key-AAAA") || strings.Contains(out, jwt) {
		t.Fatalf("key material still present: %s", out)
	}
	if !strings.Contains(out, Token) {
		t.Fatalf("expected %s, got %s", Token, out)
	}
}
