package main

import "testing"

func TestFormatTrayTitle(t *testing.T) {
	const (
		downRate   = 2 * 1024 * 1024
		proxyTotal = 9 * 1024 * 1024
	)
	for _, tc := range []struct {
		name           string
		showSpeed      bool
		showProxyTotal bool
		want           string
	}{
		{name: "neither"},
		{name: "speed", showSpeed: true, want: "↓2.00 MB/s"},
		{name: "proxy total", showProxyTotal: true, want: "9 MB"},
		{name: "both", showSpeed: true, showProxyTotal: true, want: "↓2.00 MB/s · 9 MB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTrayTitle(tc.showSpeed, downRate, tc.showProxyTotal, proxyTotal)
			if got != tc.want {
				t.Errorf("formatTrayTitle = %q, want %q", got, tc.want)
			}
		})
	}
}
