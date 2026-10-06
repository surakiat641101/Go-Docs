package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	for _, k := range []string{"PORT", "BODY_LIMIT_MB", "ALLOW_REMOTE_IMAGES", "CORS_ALLOW_ORIGINS"} {
		t.Setenv(k, "")
	}
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Port: "3000", BodyLimitMB: 20, AllowRemoteImages: false, CORSAllowOrigins: "*"}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("BODY_LIMIT_MB", "5")
	t.Setenv("ALLOW_REMOTE_IMAGES", "true")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://a.example.com")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Port: "8080", BodyLimitMB: 5, AllowRemoteImages: true, CORSAllowOrigins: "https://a.example.com"}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestFromEnvInvalid(t *testing.T) {
	t.Setenv("BODY_LIMIT_MB", "abc")
	if _, err := FromEnv(); err == nil {
		t.Error("expected error for invalid BODY_LIMIT_MB")
	}
	t.Setenv("BODY_LIMIT_MB", "20")
	t.Setenv("ALLOW_REMOTE_IMAGES", "yes please")
	if _, err := FromEnv(); err == nil {
		t.Error("expected error for invalid ALLOW_REMOTE_IMAGES")
	}
}
