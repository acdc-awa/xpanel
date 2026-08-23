package contracts

import (
	"context"
	"strings"
	"testing"
)

type fakeExporter struct {
	key    string
	uaHit  bool
	export string
}

func (f *fakeExporter) FormatKey() string { return f.key }

func (f *fakeExporter) MatchUserAgent(ua string) bool { return f.uaHit }

func (f *fakeExporter) Export(ctx context.Context, user UserSummaryDTO, nodes []ProxyNodeDTO, opts ExportOptions) (string, string, error) {
	return f.export, "text/plain", nil
}

func TestExporterRegistry_RegisterFindMatch(t *testing.T) {
	r := NewExporterRegistry()
	clash := &fakeExporter{key: "clash", uaHit: true, export: "clash-content"}
	base64 := &fakeExporter{key: "base64", uaHit: true, export: "base64-content"}
	r.Register(clash)
	r.Register(base64)

	if got := r.Find("clash"); got != clash {
		t.Fatalf("Find(clash) = %#v, want clash", got)
	}
	if got := r.Find("missing"); got != nil {
		t.Fatalf("Find(missing) = %#v, want nil", got)
	}
	if got := r.Match("clash/1.0"); got != clash {
		t.Fatalf("Match(clash/1.0) = %#v, want clash", got)
	}
}

func TestExporterRegistry_RegisterOverride(t *testing.T) {
	r := NewExporterRegistry()
	old := &fakeExporter{key: "base64", export: "old"}
	neu := &fakeExporter{key: "base64", export: "new"}
	r.Register(old)
	r.Register(neu)
	if got := r.Find("base64"); got != neu {
		t.Fatalf("Find(base64) = %#v, want new", got)
	}
}

func TestExporterRegistry_NoMatch(t *testing.T) {
	r := NewExporterRegistry()
	r.Register(&fakeExporter{key: "clash", uaHit: false})
	if got := r.Match("unknown"); got != nil {
		t.Fatalf("Match(unknown) = %#v, want nil", got)
	}
	if _, _, err := r.Export(context.Background(), "unknown", UserSummaryDTO{}, nil, ExportOptions{}); err == nil {
		t.Fatal("expected ErrNoExporterMatched")
	} else if !strings.Contains(err.Error(), "no subscription exporter matched") {
		t.Fatalf("unexpected error: %v", err)
	}
}
