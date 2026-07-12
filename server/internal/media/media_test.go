package media

import (
	"context"
	"testing"
)

func TestDataURLRoundTrip(t *testing.T) {
	orig := Image{MimeType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x01}}
	url := orig.DataURL()

	got, ok := ParseDataURL(url)
	if !ok {
		t.Fatalf("ParseDataURL(%q) returned ok=false", url)
	}
	if got.MimeType != orig.MimeType {
		t.Errorf("mime = %q, want %q", got.MimeType, orig.MimeType)
	}
	if string(got.Data) != string(orig.Data) {
		t.Errorf("data = %v, want %v", got.Data, orig.Data)
	}
}

func TestParseDataURLRejectsNonData(t *testing.T) {
	for _, s := range []string{"", "https://example.com/a.png", "data:image/png,notbase64", "notaurl"} {
		if _, ok := ParseDataURL(s); ok {
			t.Errorf("ParseDataURL(%q) = ok, want not ok", s)
		}
	}
}

func TestCollectorViaContext(t *testing.T) {
	ctx, c := WithCollector(context.Background())
	if CollectorFrom(ctx) != c {
		t.Fatal("CollectorFrom did not return the attached collector")
	}
	c.Add(Image{MimeType: "image/png", Data: []byte("a")})
	c.Add(Image{MimeType: "image/png", Data: []byte("b")})
	if got := c.Images(); len(got) != 2 {
		t.Fatalf("Images len = %d, want 2", len(got))
	}
	// Absent collector is nil, not a panic.
	if CollectorFrom(context.Background()) != nil {
		t.Error("CollectorFrom on bare context should be nil")
	}
}

func TestInboundViaContext(t *testing.T) {
	if Inbound(context.Background()) != nil {
		t.Error("Inbound on bare context should be nil")
	}
	img := &Image{MimeType: "image/jpeg", Data: []byte("x")}
	ctx := WithInbound(context.Background(), img)
	if Inbound(ctx) != img {
		t.Error("Inbound did not return the attached image")
	}
}
