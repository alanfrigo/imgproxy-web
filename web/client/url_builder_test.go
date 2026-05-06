package client

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSpec_BuildScalars(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want string
	}{
		{
			name: "format and quality",
			opts: map[string]any{"format": "webp", "quality": 85.0},
			want: "f:webp/q:85",
		},
		{
			name: "bool true",
			opts: map[string]any{"strip_metadata": true},
			want: "sm:1",
		},
		{
			name: "bool false skipped",
			opts: map[string]any{"strip_metadata": false},
			want: "",
		},
		{
			name: "zero number skipped",
			opts: map[string]any{"width": 0.0},
			want: "",
		},
		{
			name: "size + dpr",
			opts: map[string]any{"width": 800.0, "height": 600.0, "dpr": 2.0, "resizing_type": "fit"},
			want: "dpr:2/h:600/rt:fit/w:800",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Spec{Options: tc.opts}
			got, err := s.Build()
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSpec_BuildCompounds(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want string
	}{
		{
			name: "gravity simple",
			opts: map[string]any{"gravity": map[string]any{"type": "ce"}},
			want: "g:ce",
		},
		{
			name: "gravity focus point",
			opts: map[string]any{"gravity": map[string]any{"type": "fp", "x": "0.5", "y": "0.3"}},
			want: "g:fp:0.5:0.3",
		},
		{
			name: "crop with gravity",
			opts: map[string]any{"crop": map[string]any{
				"width":   100.0,
				"height":  200.0,
				"gravity": map[string]any{"type": "no"},
			}},
			want: "c:100:200:no",
		},
		{
			name: "padding 4 values",
			opts: map[string]any{"padding": []any{10.0, 20.0, 10.0, 20.0}},
			want: "pd:10:20:10:20",
		},
		{
			name: "flip horizontal",
			opts: map[string]any{"flip": "horizontal"},
			want: "fl:1:0",
		},
		{
			name: "format_quality pairs",
			opts: map[string]any{"format_quality": []any{
				map[string]any{"format": "jpeg", "quality": 90.0},
				map[string]any{"format": "webp", "quality": 80.0},
			}},
			want: "fq:jpeg:90:webp:80",
		},
		{
			name: "extend enabled",
			opts: map[string]any{"extend": map[string]any{
				"enabled": true,
				"gravity": map[string]any{"type": "so"},
			}},
			want: "ex:1:so",
		},
		{
			name: "trim threshold + color",
			opts: map[string]any{"trim": map[string]any{
				"threshold": 10.0,
				"color":     "#ff0000",
				"equal_h":   true,
				"equal_v":   false,
			}},
			want: "t:10:ff0000:1:0",
		},
		{
			name: "watermark",
			opts: map[string]any{"watermark": map[string]any{
				"opacity": 0.5,
				"gravity": "so",
				"x":       10.0,
				"y":       20.0,
				"scale":   0.3,
			}},
			want: "wm:0.5:so:10:20:0.3",
		},
		{
			name: "skip_processing list",
			opts: map[string]any{"skip_processing": []any{"gif", "svg"}},
			want: "skp:gif:svg",
		},
		{
			name: "filename base64-encoded",
			// "myfile" base64url no padding = "bXlmaWxl"
			opts: map[string]any{"filename": "myfile"},
			want: "fn:" + base64.RawURLEncoding.EncodeToString([]byte("myfile")) + ":1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Spec{Options: tc.opts}
			got, err := s.Build()
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSpec_RawWins(t *testing.T) {
	s := &Spec{
		Raw:     "/rs:fit:800:600/q:85/",
		Options: map[string]any{"format": "ignored"},
	}
	got, err := s.Build()
	if err != nil {
		t.Fatal(err)
	}
	if got != "rs:fit:800:600/q:85" {
		t.Fatalf("got %q", got)
	}
}

func TestSpec_OutputExtension(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"options format", Spec{Options: map[string]any{"format": "WebP"}}, "webp"},
		{"raw f:", Spec{Raw: "rs:fit:100:100/f:avif"}, "avif"},
		{"raw @suffix", Spec{Raw: "plain/http://x/y.jpg@png"}, "png"},
		{"empty", Spec{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.spec.OutputExtension(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSpec_UnknownOption(t *testing.T) {
	s := &Spec{Options: map[string]any{"nonexistent": 1}}
	if _, err := s.Build(); err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("want unknown option error, got %v", err)
	}
}

func TestSigner_DisabledWhenEmpty(t *testing.T) {
	sg, err := NewSigner("", "", 32)
	if err != nil {
		t.Fatal(err)
	}
	if sg != nil {
		t.Fatalf("want nil signer, got %v", sg)
	}
	if got := (*Signer)(nil).Sign("/foo"); got != "_" {
		t.Fatalf("nil signer should return _, got %q", got)
	}
}

func TestSigner_KnownVector(t *testing.T) {
	// Mirrors imgproxy's security/signature_test.go:
	// key=raw "test-key" (hex 746573742d6b6579), salt=raw "test-salt"
	// (hex 746573742d73616c74), path="asd" → "dtLwhdnPPiu_epMl1LrzheLpvHas-4mwvY6L3Z8WwlY".
	sg, err := NewSigner(
		"746573742d6b6579",
		"746573742d73616c74",
		32,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := sg.Sign("asd")
	want := "dtLwhdnPPiu_epMl1LrzheLpvHas-4mwvY6L3Z8WwlY"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSigner_TruncatedSize(t *testing.T) {
	sg, err := NewSigner(
		"746573742d6b6579",
		"746573742d73616c74",
		8,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := sg.Sign("asd")
	// 8 bytes base64url no padding = 11 chars; matches imgproxy's truncated test.
	want := "dtLwhdnPPis"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
