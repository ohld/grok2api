package web

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestVideoPacerSerializesAndSpacesPerNode(t *testing.T) {
	var pacer videoPacer
	spacing := 150 * time.Millisecond
	first, err := pacer.acquire(context.Background(), 7, spacing)
	if err != nil {
		t.Fatal(err)
	}
	// Another node is independent.
	other, err := pacer.acquire(context.Background(), 8, spacing)
	if err != nil {
		t.Fatal(err)
	}
	other.release()
	// Same node is busy: a short deadline must give up.
	busyCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if _, err := pacer.acquire(busyCtx, 7, spacing); err == nil {
		t.Fatal("second video on a busy node must wait")
	}
	cancel()
	first.started()
	first.release()
	began := time.Now()
	second, err := pacer.acquire(context.Background(), 7, spacing)
	if err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(began); waited < 100*time.Millisecond {
		t.Fatalf("waited %v, want spacing after a started video", waited)
	}
	// A create that never started does not push the next one back.
	second.release()
	began = time.Now()
	third, err := pacer.acquire(context.Background(), 7, 0)
	if err != nil || time.Since(began) > 50*time.Millisecond {
		t.Fatalf("zero spacing must not wait: %v", err)
	}
	third.release()
}

func TestMain(m *testing.M) {
	webVideoDecoyReadyWithin = 0
	os.Exit(m.Run())
}

func TestDecoyVideoTiming(t *testing.T) {
	defer func(previous time.Duration) { webVideoDecoyReadyWithin = previous }(webVideoDecoyReadyWithin)
	webVideoDecoyReadyWithin = 10 * time.Second
	if !isDecoyVideoTiming(3*time.Second) || isDecoyVideoTiming(55*time.Second) {
		t.Fatal("a 3s finish is a decoy, a 55s render is not")
	}
}
