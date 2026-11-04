package app

import (
	"testing"
	"time"
)

func TestDeliveryBackoffBounded(t *testing.T) {
	if delay := deliveryBackoff(50, 0); delay <= 0 || delay > 6*time.Hour {
		t.Fatalf("delay = %v", delay)
	}
	if delay := deliveryBackoff(2, 2*time.Minute); delay != 2*time.Minute {
		t.Fatalf("retry_after override = %v", delay)
	}
}
