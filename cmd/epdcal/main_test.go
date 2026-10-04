package main

import "testing"

func TestLoopbackAddress(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{listen: "0.0.0.0:8080", want: "127.0.0.1:8080"},
		{listen: ":8080", want: "127.0.0.1:8080"},
		{listen: "[::]:8080", want: "127.0.0.1:8080"},
		{listen: "127.0.0.1:8080", want: "127.0.0.1:8080"},
		{listen: "localhost:8080", want: "localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.listen, func(t *testing.T) {
			if got := loopbackAddress(tt.listen); got != tt.want {
				t.Fatalf("loopbackAddress(%q) = %q, want %q", tt.listen, got, tt.want)
			}
		})
	}
}
