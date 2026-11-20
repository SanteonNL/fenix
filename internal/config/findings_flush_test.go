package config

import (
	"testing"
	"time"
)

func TestFindingsFlushEvery(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 30 * time.Second, "10s": 10 * time.Second, "2m": 2 * time.Minute, "off": 0, "OFF": 0} {
		got, err := OutputConfig{Findings: FindingsConfig{FlushEvery: in}}.FindingsFlushEvery()
		if err != nil || got != want {
			t.Errorf("%q: %v %v, want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"soon", "-5s", "0s"} {
		if _, err := (OutputConfig{Findings: FindingsConfig{FlushEvery: bad}}).FindingsFlushEvery(); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
