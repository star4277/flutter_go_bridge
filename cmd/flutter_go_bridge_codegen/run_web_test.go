package main

import "testing"

func TestIsWebFlutterRun(t *testing.T) {
	tests := []struct {
		name   string
		device string
		args   []string
		want   bool
	}{
		{name: "chrome shorthand", device: "chrome", want: true},
		{name: "web server", device: "web-server", want: true},
		{name: "native device", device: "windows", want: false},
		{name: "forwarded device", args: []string{"-d", "edge"}, want: true},
		{name: "forwarded native device", args: []string{"--device-id", "9654457421001D"}, want: false},
		{name: "wasm flag", args: []string{"--wasm"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isWebFlutterRun(test.device, test.args); got != test.want {
				t.Fatalf("isWebFlutterRun(%q, %#v) = %v, want %v", test.device, test.args, got, test.want)
			}
		})
	}
}
