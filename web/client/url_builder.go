package client

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Spec is the user-supplied processing description.
//
// Either Raw or Options should be set. Raw wins when both are present.
type Spec struct {
	// Raw is a fully-formed imgproxy options string like "rs:fit:800:600/q:85".
	// Leading and trailing slashes are stripped before use.
	Raw string `json:"raw,omitempty"`
	// Options is a key→value map keyed by schema.Option.Key.
	Options map[string]any `json:"options,omitempty"`
	// Filename is the desired output filename (without extension); used by the
	// server for ZIP entries and Content-Disposition. Not part of the URL.
	Filename string `json:"filename,omitempty"`
}

// Build returns the imgproxy options path segment (no leading/trailing slash).
func (s *Spec) Build() (string, error) {
	if s == nil {
		return "", nil
	}
	if raw := strings.Trim(strings.TrimSpace(s.Raw), "/"); raw != "" {
		return raw, nil
	}
	parts := make([]string, 0, len(s.Options))
	keys := make([]string, 0, len(s.Options))
	for k := range s.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		seg, err := encodeOption(k, s.Options[k])
		if err != nil {
			return "", fmt.Errorf("option %q: %w", k, err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "/"), nil
}

// OutputExtension returns the format suffix that should drive the output
// filename, derived from the Spec. Empty means "keep source".
func (s *Spec) OutputExtension() string {
	if s == nil {
		return ""
	}
	if s.Options != nil {
		if v, ok := s.Options["format"]; ok {
			return strings.TrimPrefix(strings.ToLower(asString(v)), ".")
		}
	}
	// Try to sniff format from raw.
	if s.Raw != "" {
		for _, seg := range strings.Split(strings.Trim(s.Raw, "/"), "/") {
			if strings.HasPrefix(seg, "f:") || strings.HasPrefix(seg, "format:") || strings.HasPrefix(seg, "ext:") {
				if i := strings.Index(seg, ":"); i >= 0 && i+1 < len(seg) {
					return strings.ToLower(seg[i+1:])
				}
			}
			if i := strings.LastIndex(seg, "@"); i > 0 {
				return strings.ToLower(seg[i+1:])
			}
		}
	}
	return ""
}

// encodeOption renders a single option key + value into "name:arg:arg" form.
// Returns "" when the value is empty/zero and should be skipped.
func encodeOption(key string, val any) (string, error) {
	short, ok := optionShort[key]
	if !ok {
		return "", fmt.Errorf("unknown option")
	}
	switch key {
	case "gravity":
		return encodeGravity(short, val)
	case "crop":
		return encodeCrop(short, val)
	case "extend", "extend_aspect_ratio":
		return encodeExtend(short, val)
	case "trim":
		return encodeTrim(short, val)
	case "padding":
		return encodePadding(short, val)
	case "watermark":
		return encodeWatermark(short, val)
	case "flip":
		return encodeFlip(short, val)
	case "format_quality":
		return encodeFmtQual(short, val)
	case "filename":
		return encodeFilename(short, val)
	case "skip_processing":
		return encodeStringList(short, val)
	}
	// Scalars.
	switch v := val.(type) {
	case nil:
		return "", nil
	case bool:
		if !v {
			return "", nil
		}
		return short + ":1", nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", nil
		}
		return short + ":" + s, nil
	case float64:
		if v == 0 && !alwaysEmitZero[key] {
			return "", nil
		}
		return short + ":" + trimFloat(v), nil
	case json_number:
		return short + ":" + string(v), nil
	}
	return "", fmt.Errorf("unsupported value type %T", val)
}

// json_number lets callers pass json.Number directly without depending on the
// encoding/json package here.
type json_number string

func encodeGravity(short string, val any) (string, error) {
	g, ok := val.(map[string]any)
	if !ok || g == nil {
		return "", nil
	}
	t := strings.TrimSpace(asString(g["type"]))
	if t == "" {
		return "", nil
	}
	parts := []string{short, t}
	if x, ok := g["x"]; ok && x != nil && asString(x) != "" {
		parts = append(parts, asString(x))
		if y, ok := g["y"]; ok && y != nil && asString(y) != "" {
			parts = append(parts, asString(y))
		}
	}
	return strings.Join(parts, ":"), nil
}

func encodeCrop(short string, val any) (string, error) {
	c, ok := val.(map[string]any)
	if !ok || c == nil {
		return "", nil
	}
	w := asString(c["width"])
	h := asString(c["height"])
	if w == "" && h == "" {
		return "", nil
	}
	if w == "" {
		w = "0"
	}
	if h == "" {
		h = "0"
	}
	parts := []string{short, w, h}
	if g, ok := c["gravity"].(map[string]any); ok {
		if t := asString(g["type"]); t != "" {
			parts = append(parts, t)
			if x := asString(g["x"]); x != "" {
				parts = append(parts, x)
				if y := asString(g["y"]); y != "" {
					parts = append(parts, y)
				}
			}
		}
	}
	return strings.Join(parts, ":"), nil
}

func encodeExtend(short string, val any) (string, error) {
	e, ok := val.(map[string]any)
	if !ok || e == nil {
		return "", nil
	}
	enabled := asBool(e["enabled"])
	if !enabled {
		return "", nil
	}
	parts := []string{short, "1"}
	if g, ok := e["gravity"].(map[string]any); ok {
		if t := asString(g["type"]); t != "" {
			parts = append(parts, t)
			if x := asString(g["x"]); x != "" {
				parts = append(parts, x)
				if y := asString(g["y"]); y != "" {
					parts = append(parts, y)
				}
			}
		}
	}
	return strings.Join(parts, ":"), nil
}

func encodeTrim(short string, val any) (string, error) {
	t, ok := val.(map[string]any)
	if !ok || t == nil {
		return "", nil
	}
	thr := asString(t["threshold"])
	if thr == "" {
		return "", nil
	}
	parts := []string{short, thr}
	if c := asString(t["color"]); c != "" {
		parts = append(parts, strings.TrimPrefix(c, "#"))
		if eh, ok := t["equal_h"]; ok && eh != nil {
			parts = append(parts, boolStr(asBool(eh)))
			if ev, ok := t["equal_v"]; ok && ev != nil {
				parts = append(parts, boolStr(asBool(ev)))
			}
		}
	}
	return strings.Join(parts, ":"), nil
}

func encodePadding(short string, val any) (string, error) {
	a, ok := val.([]any)
	if !ok || len(a) == 0 {
		return "", nil
	}
	if len(a) != 1 && len(a) != 2 && len(a) != 4 {
		return "", errors.New("padding must have 1, 2 or 4 values")
	}
	parts := []string{short}
	for _, v := range a {
		s := asString(v)
		if s == "" {
			s = "0"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ":"), nil
}

func encodeWatermark(short string, val any) (string, error) {
	w, ok := val.(map[string]any)
	if !ok || w == nil {
		return "", nil
	}
	op := asString(w["opacity"])
	if op == "" {
		return "", nil
	}
	parts := []string{short, op}
	if g := asString(w["gravity"]); g != "" {
		parts = append(parts, g)
		x := asString(w["x"])
		y := asString(w["y"])
		if x != "" || y != "" {
			if x == "" {
				x = "0"
			}
			if y == "" {
				y = "0"
			}
			parts = append(parts, x, y)
			if s := asString(w["scale"]); s != "" {
				parts = append(parts, s)
			}
		} else if s := asString(w["scale"]); s != "" {
			parts = append(parts, "0", "0", s)
		}
	}
	return strings.Join(parts, ":"), nil
}

func encodeFlip(short string, val any) (string, error) {
	s := strings.ToLower(asString(val))
	switch s {
	case "", "none":
		return "", nil
	case "h", "horizontal":
		return short + ":1:0", nil
	case "v", "vertical":
		return short + ":0:1", nil
	case "both":
		return short + ":1:1", nil
	}
	return "", fmt.Errorf("invalid flip %q", s)
}

func encodeFmtQual(short string, val any) (string, error) {
	a, ok := val.([]any)
	if !ok || len(a) == 0 {
		return "", nil
	}
	parts := []string{short}
	for _, item := range a {
		m, ok := item.(map[string]any)
		if !ok {
			return "", errors.New("format_quality entries must be objects")
		}
		f := strings.TrimSpace(asString(m["format"]))
		q := strings.TrimSpace(asString(m["quality"]))
		if f == "" || q == "" {
			continue
		}
		parts = append(parts, f, q)
	}
	if len(parts) == 1 {
		return "", nil
	}
	return strings.Join(parts, ":"), nil
}

func encodeFilename(short string, val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case string:
		if v == "" {
			return "", nil
		}
		return short + ":" + base64.RawURLEncoding.EncodeToString([]byte(v)) + ":1", nil
	case map[string]any:
		name := asString(v["name"])
		if name == "" {
			return "", nil
		}
		if asBool(v["base64"]) {
			return short + ":" + base64.RawURLEncoding.EncodeToString([]byte(name)) + ":1", nil
		}
		return short + ":" + name, nil
	}
	return "", fmt.Errorf("unsupported filename value")
}

func encodeStringList(short string, val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case []any:
		if len(v) == 0 {
			return "", nil
		}
		parts := []string{short}
		for _, item := range v {
			if s := strings.TrimSpace(asString(item)); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 1 {
			return "", nil
		}
		return strings.Join(parts, ":"), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", nil
		}
		return short + ":" + strings.ReplaceAll(s, ",", ":"), nil
	}
	return "", fmt.Errorf("unsupported list value")
}

// asString coerces common JSON scalar types to a stripped string.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	case float64:
		return trimFloat(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case json_number:
		return string(t)
	}
	return fmt.Sprint(v)
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(t))
		return b
	case float64:
		return t != 0
	}
	return false
}

func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// optionShort maps schema keys to imgproxy short URL names.
var optionShort = map[string]string{
	"format":                         "f",
	"quality":                        "q",
	"format_quality":                 "fq",
	"max_bytes":                      "mb",
	"resizing_type":                  "rt",
	"width":                          "w",
	"height":                         "h",
	"min_width":                      "mw",
	"min_height":                     "mh",
	"dpr":                            "dpr",
	"zoom":                           "z",
	"enlarge":                        "el",
	"extend":                         "ex",
	"extend_aspect_ratio":            "exar",
	"gravity":                        "g",
	"crop":                           "c",
	"padding":                        "pd",
	"trim":                           "t",
	"rotate":                         "rot",
	"auto_rotate":                    "ar",
	"flip":                           "fl",
	"background":                     "bg",
	"blur":                           "bl",
	"sharpen":                        "sh",
	"pixelate":                       "pix",
	"watermark":                      "wm",
	"strip_metadata":                 "sm",
	"keep_copyright":                 "kcr",
	"strip_color_profile":            "scp",
	"enforce_thumbnail":              "eth",
	"filename":                       "fn",
	"return_attachment":              "att",
	"raw":                            "raw",
	"cachebuster":                    "cb",
	"expires":                        "exp",
	"skip_processing":                "skp",
	"preset":                         "pr",
	"max_src_resolution":             "msr",
	"max_src_file_size":              "msfs",
	"max_animation_frames":           "maf",
	"max_animation_frame_resolution": "mafr",
	"max_result_dimension":           "mrd",
}

// alwaysEmitZero forces emitting numeric options even when zero. Currently no
// option requires this — kept as an escape hatch.
var alwaysEmitZero = map[string]bool{}

// EncodeSourceURL encodes the source URL using imgproxy's base64url scheme.
func EncodeSourceURL(src string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(src))
}

// PlainSourcePath wraps a source URL using the /plain/ scheme so colons in URLs
// don't collide with the option separator.
func PlainSourcePath(src string) string {
	return "plain/" + src
}
