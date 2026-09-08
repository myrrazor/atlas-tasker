package service

import "testing"

func TestSecretLikeFindings(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{text: "hello", want: ""},
		{text: "token ghp_abcdefghijklmnopqrstuvwx", want: "github_pat"},
		{text: "-----BEGIN RSA PRIVATE KEY-----\nhidden\n-----END RSA PRIVATE KEY-----", want: "private_key_pem"},
		{text: "AKIAIOSFODNN7EXAMPLE", want: "aws_access_key"},
	}
	for _, tc := range cases {
		got := SecretLikeFindings(tc.text)
		if tc.want == "" {
			if len(got) != 0 {
				t.Fatalf("%q: want no findings, got %v", tc.text, got)
			}
			continue
		}
		found := false
		for _, item := range got {
			if item == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q: want %q in %v", tc.text, tc.want, got)
		}
	}
}
