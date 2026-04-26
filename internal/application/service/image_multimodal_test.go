package service

import (
	"os"
	"testing"
	"time"
)

func TestVLMRetryConfig(t *testing.T) {
	origAttempts := os.Getenv("WEKNORA_VLM_RETRY_ATTEMPTS")
	origDelay := os.Getenv("WEKNORA_VLM_RETRY_DELAY_MS")
	defer func() {
		if origAttempts == "" {
			_ = os.Unsetenv("WEKNORA_VLM_RETRY_ATTEMPTS")
		} else {
			_ = os.Setenv("WEKNORA_VLM_RETRY_ATTEMPTS", origAttempts)
		}
		if origDelay == "" {
			_ = os.Unsetenv("WEKNORA_VLM_RETRY_DELAY_MS")
		} else {
			_ = os.Setenv("WEKNORA_VLM_RETRY_DELAY_MS", origDelay)
		}
	}()

	_ = os.Unsetenv("WEKNORA_VLM_RETRY_ATTEMPTS")
	_ = os.Unsetenv("WEKNORA_VLM_RETRY_DELAY_MS")
	if got := getVLMRetryAttempts(); got != 3 {
		t.Fatalf("default attempts = %d, want 3", got)
	}
	if got := getVLMRetryDelay(); got != 2*time.Second {
		t.Fatalf("default delay = %v, want 2s", got)
	}

	_ = os.Setenv("WEKNORA_VLM_RETRY_ATTEMPTS", "5")
	_ = os.Setenv("WEKNORA_VLM_RETRY_DELAY_MS", "1500")
	if got := getVLMRetryAttempts(); got != 5 {
		t.Fatalf("configured attempts = %d, want 5", got)
	}
	if got := getVLMRetryDelay(); got != 1500*time.Millisecond {
		t.Fatalf("configured delay = %v, want 1500ms", got)
	}
}
