package vlm

import (
	"os"
	"testing"
	"time"
)

func TestGetVLMTimeout(t *testing.T) {
	original := os.Getenv("WEKNORA_VLM_TIMEOUT_SECONDS")
	defer func() {
		if original == "" {
			_ = os.Unsetenv("WEKNORA_VLM_TIMEOUT_SECONDS")
		} else {
			_ = os.Setenv("WEKNORA_VLM_TIMEOUT_SECONDS", original)
		}
	}()

	_ = os.Unsetenv("WEKNORA_VLM_TIMEOUT_SECONDS")
	if got := getVLMTimeout(); got != defaultTimeout {
		t.Fatalf("default timeout = %v, want %v", got, defaultTimeout)
	}

	_ = os.Setenv("WEKNORA_VLM_TIMEOUT_SECONDS", "300")
	if got := getVLMTimeout(); got != 300*time.Second {
		t.Fatalf("configured timeout = %v, want %v", got, 300*time.Second)
	}

	_ = os.Setenv("WEKNORA_VLM_TIMEOUT_SECONDS", "bad")
	if got := getVLMTimeout(); got != defaultTimeout {
		t.Fatalf("invalid timeout fallback = %v, want %v", got, defaultTimeout)
	}
}
