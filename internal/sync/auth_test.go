package sync

import "testing"

func TestShouldPromptForSSHPassword(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "permission denied", err: testError("Permission denied (publickey,password)."), want: true},
		{name: "keyboard interactive", err: testError("no supported authentication methods available (server sent: keyboard-interactive)"), want: true},
		{name: "host unreachable", err: testError("ssh: connect to host example.com port 22: No route to host"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPromptForSSHPassword(tt.err); got != tt.want {
				t.Fatalf("shouldPromptForSSHPassword(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type testError string

func (e testError) Error() string { return string(e) }