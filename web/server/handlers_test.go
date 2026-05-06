package server

import (
	"testing"

	"github.com/imgproxy/imgproxy/v3/web/client"
)

func TestOutputName(t *testing.T) {
	cases := []struct {
		name string
		job  job
		idx  int
		want string
	}{
		{
			name: "default keeps stem and applies new ext",
			job:  job{origName: "photo.jpg", spec: &client.Spec{Options: map[string]any{"format": "webp"}}},
			idx:  0,
			want: "photo.webp",
		},
		{
			name: "no format keeps source ext",
			job:  job{origName: "photo.PNG", spec: &client.Spec{}},
			idx:  0,
			want: "photo.png",
		},
		{
			name: "jpeg normalized to jpg",
			job:  job{origName: "x.bmp", spec: &client.Spec{Options: map[string]any{"format": "jpeg"}}},
			idx:  0,
			want: "x.jpg",
		},
		{
			name: "template index padded",
			job: job{
				origName: "praia.jpg",
				spec: &client.Spec{
					Options:          map[string]any{"format": "webp"},
					FilenameTemplate: "{i:03d}-{name}.{ext}",
				},
			},
			idx:  4,
			want: "005-praia.webp",
		},
		{
			name: "template index plain",
			job: job{
				origName: "a.jpg",
				spec: &client.Spec{
					Options:          map[string]any{"format": "avif"},
					FilenameTemplate: "out-{i}.{ext}",
				},
			},
			idx:  0,
			want: "out-1.avif",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := outputName(tc.job, tc.idx)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
