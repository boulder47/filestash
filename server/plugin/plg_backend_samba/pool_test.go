package plg_backend_samba

import "testing"

func TestShouldProbe(t *testing.T) {
	const threshold = sambaIdleProbeSeconds // 30
	cases := []struct {
		name               string
		lastUsed, now, thr int64
		want               bool
	}{
		{"hot session skips probe", 1000, 1000 + 5, threshold, false},
		{"just under threshold", 1000, 1000 + 29, threshold, false},
		{"exactly at threshold probes", 1000, 1000 + 30, threshold, true},
		{"long idle probes", 1000, 1000 + 3600, threshold, true},
		{"never used (zero) probes", 0, 1000, threshold, true},
	}
	for _, c := range cases {
		if got := shouldProbe(c.lastUsed, c.now, c.thr); got != c.want {
			t.Errorf("%s: shouldProbe(%d,%d,%d)=%v want %v", c.name, c.lastUsed, c.now, c.thr, got, c.want)
		}
	}
}
