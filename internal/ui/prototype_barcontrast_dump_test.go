// PROTOTYPE bar-contrast: failure breakdown. go test ./internal/ui -run TestProtoBreakdown -v
package ui

import "testing"

func TestProtoBreakdown(t *testing.T) {
	ProtoPaletteKnown = true
	defer func() { ProtoPaletteKnown = false }()
	for v, p := range protoVariants {
		by := map[string]int{}
		for _, c := range protoCases() {
			_, r := protoBarLine(c, v, 60)
			if r.min() < 4.5 {
				by[c.section]++
				if v > 0 {
					t.Logf("%s  %-34s %-7s mode %.1f mid %.1f title %.1f", p.Key, c.label, c.mode, r.mode, r.mid, r.title)
				}
			}
		}
		t.Logf("== %s %s: %v", p.Key, p.Name, by)
	}
}
