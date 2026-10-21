package xray_test

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/xray"
)

func TestValidateTLSCertSource(t *testing.T) {
	certID := uint64(9)
	zero := uint64(0)
	cases := []struct {
		name     string
		stream   string
		certID   *uint64
		protocol string
		wantErr  string // 空 = 期望通过
	}{
		{
			name:   "managed cert satisfies",
			stream: `{"security":"tls","tlsSettings":{}}`,
			certID: &certID,
		},
		{
			name:   "explicit file pair satisfies",
			stream: `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/c.pem","keyFile":"/k.pem"}]}}`,
		},
		{
			name:   "explicit inline pair satisfies",
			stream: `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["-----BEGIN-----"],"key":["-----BEGIN-----"]}]}}`,
		},
		{
			name:    "no source rejected",
			stream:  `{"security":"tls","tlsSettings":{"serverName":"a.com"}}`,
			wantErr: "必须绑定托管证书",
		},
		{
			name:    "only verify rejected",
			stream:  `{"security":"tls","tlsSettings":{"certificates":[{"usage":"verify","certificateFile":"/ca.crt"}]}}`,
			wantErr: "必须绑定托管证书",
		},
		{
			name:    "cert without key rejected",
			stream:  `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/c.pem"}]}}`,
			wantErr: "必须绑定托管证书",
		},
		{
			name:    "cert id zero is unbound",
			stream:  `{"security":"tls","tlsSettings":{}}`,
			certID:  &zero,
			wantErr: "必须绑定托管证书",
		},
		{
			name:   "non-tls ignored",
			stream: `{"security":"none"}`,
		},
		{
			name:   "empty stream ignored",
			stream: "",
		},
		{
			name:     "hysteria requires cert even without stream",
			stream:   "",
			protocol: "hysteria",
			wantErr:  "必须绑定托管证书",
		},
		{
			name:     "hysteria managed cert satisfies",
			stream:   "",
			protocol: "hysteria",
			certID:   &certID,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := xray.ValidateTLSCertSource(tc.stream, tc.certID, tc.protocol)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}
