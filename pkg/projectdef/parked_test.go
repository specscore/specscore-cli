package projectdef

import "testing"

func TestEffectiveParkedStaleDays_DefaultWhenNoParkedBlock(t *testing.T) {
	cfg := parseCfg(t, "project:\n  title: X\n")
	if got := cfg.EffectiveParkedStaleDays(); got != DefaultParkedStaleDays {
		t.Fatalf("got %d, want default %d", got, DefaultParkedStaleDays)
	}
}

func TestEffectiveParkedStaleDays_ConfiguredOverride(t *testing.T) {
	cfg := parseCfg(t, "parked:\n  stale_days: 30\n")
	if got := cfg.EffectiveParkedStaleDays(); got != 30 {
		t.Fatalf("got %d, want 30", got)
	}
}

func TestEffectiveParkedStaleDays_ZeroFallsBackToDefault(t *testing.T) {
	cfg := parseCfg(t, "parked:\n  stale_days: 0\n")
	if got := cfg.EffectiveParkedStaleDays(); got != DefaultParkedStaleDays {
		t.Fatalf("got %d, want default %d", got, DefaultParkedStaleDays)
	}
}

func TestEffectiveParkedStaleDays_NegativeFallsBackToDefault(t *testing.T) {
	cfg := parseCfg(t, "parked:\n  stale_days: -5\n")
	if got := cfg.EffectiveParkedStaleDays(); got != DefaultParkedStaleDays {
		t.Fatalf("got %d, want default %d", got, DefaultParkedStaleDays)
	}
}
