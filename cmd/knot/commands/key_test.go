package commands

import "testing"

func TestIsPrivateKeyEndLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "openssh private key footer", line: "-----END OPENSSH PRIVATE KEY-----", want: true},
		{name: "rsa private key footer", line: "-----END RSA PRIVATE KEY-----", want: true},
		{name: "pkcs8 private key footer", line: "-----END PRIVATE KEY-----", want: true},
		{name: "trim whitespace", line: "  -----END EC PRIVATE KEY-----  ", want: true},
		{name: "base64 body containing end", line: "abcENDdef", want: false},
		{name: "public key footer", line: "-----END PUBLIC KEY-----", want: false},
		{name: "non-footer text", line: "PRIVATE KEY END", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPrivateKeyEndLine(tt.line); got != tt.want {
				t.Fatalf("isPrivateKeyEndLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
