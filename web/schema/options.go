// Package schema describes the full set of imgproxy processing options exposed
// by imgproxy-web. The catalog drives both the UI form (served at /api/options)
// and the URL builder used by the server when calling upstream imgproxy.
package schema

// Control identifies how a UI input should be rendered.
type Control string

const (
	ControlNumber  Control = "number"
	ControlText    Control = "text"
	ControlBool    Control = "bool"
	ControlSelect  Control = "select"
	ControlColor   Control = "color"   // hex like #ffffff
	ControlPairs   Control = "pairs"   // list of key:value (e.g. format_quality)
	ControlList    Control = "list"    // list of strings (e.g. skip_processing)
	ControlGravity Control = "gravity" // compound: type + offsets
	ControlCrop    Control = "crop"    // compound: w,h,gravity,x,y
	ControlExtend  Control = "extend"  // compound: bool + gravity + offsets
	ControlTrim    Control = "trim"    // compound: threshold,color,equal_h,equal_v
	ControlPadding Control = "padding" // 1, 2 or 4 ints
	ControlFlip    Control = "flip"    // h, v or both
	ControlWmark   Control = "watermark"
	ControlSize    Control = "size"      // w,h,enlarge
	ControlFmtQual Control = "fmt_qual"  // pairs format:quality
	ControlFilename Control = "filename" // string + optional base64 flag
)

// Option describes a single processing option.
type Option struct {
	// Key is the name used in the JSON Spec.Options map.
	Key string `json:"key"`
	// Label is the human label shown in the UI.
	Label string `json:"label"`
	// ImgproxyName is the long option name in URLs (e.g. "resize").
	ImgproxyName string `json:"imgproxy_name"`
	// Short is the short URL name (e.g. "rs"). Used by the URL builder.
	Short string `json:"short"`
	// Control is the UI control kind.
	Control Control `json:"control"`
	// Description is help text shown next to the input.
	Description string `json:"description,omitempty"`
	// Choices populates a select control.
	Choices []Choice `json:"choices,omitempty"`
	// Min, Max bound a number control. Zero values mean unbounded.
	Min *float64 `json:"min,omitempty"`
	// Max upper bound for number controls.
	Max *float64 `json:"max,omitempty"`
	// Step for number inputs.
	Step *float64 `json:"step,omitempty"`
	// Placeholder hint text for the input.
	Placeholder string `json:"placeholder,omitempty"`
	// Default suggested default (string form).
	Default string `json:"default,omitempty"`
}

// Choice is one option in a select control.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Group is a UI accordion section.
type Group struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Options     []Option `json:"options"`
}

// Catalog is the full UI/options catalog.
type Catalog struct {
	Groups       []Group  `json:"groups"`
	Formats      []Choice `json:"formats"`
	Gravities    []Choice `json:"gravities"`
	ResizeTypes  []Choice `json:"resize_types"`
	WebPPresets  []Choice `json:"webp_presets"`
}

func ptr[T any](v T) *T { return &v }

// Build returns the static catalog. It mirrors imgproxy's processing options.
func Build() Catalog {
	return Catalog{
		Formats:     formats,
		Gravities:   gravities,
		ResizeTypes: resizeTypes,
		WebPPresets: webpPresets,
		Groups: []Group{
			{
				ID:    "output",
				Label: "Output",
				Options: []Option{
					{Key: "format", Label: "Format", ImgproxyName: "format", Short: "f", Control: ControlSelect, Choices: formats, Description: "Output image format. Leave empty to keep source format."},
					{Key: "quality", Label: "Quality", ImgproxyName: "quality", Short: "q", Control: ControlNumber, Min: ptr(0.0), Max: ptr(100.0), Step: ptr(1.0), Description: "0-100. Overrides format-specific defaults."},
					{Key: "format_quality", Label: "Per-format quality", ImgproxyName: "format_quality", Short: "fq", Control: ControlFmtQual, Description: "Pairs like jpeg:90, webp:80, avif:65."},
					{Key: "max_bytes", Label: "Max bytes", ImgproxyName: "max_bytes", Short: "mb", Control: ControlNumber, Min: ptr(0.0), Description: "Cap output file size in bytes."},
				},
			},
			{
				ID:    "resize",
				Label: "Resize",
				Options: []Option{
					{Key: "resizing_type", Label: "Resizing type", ImgproxyName: "resizing_type", Short: "rt", Control: ControlSelect, Choices: resizeTypes, Default: "fit"},
					{Key: "width", Label: "Width", ImgproxyName: "width", Short: "w", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "height", Label: "Height", ImgproxyName: "height", Short: "h", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "min_width", Label: "Min width", ImgproxyName: "min-width", Short: "mw", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "min_height", Label: "Min height", ImgproxyName: "min-height", Short: "mh", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "dpr", Label: "DPR", ImgproxyName: "dpr", Short: "dpr", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1), Description: "Device pixel ratio multiplier."},
					{Key: "zoom", Label: "Zoom", ImgproxyName: "zoom", Short: "z", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1)},
					{Key: "enlarge", Label: "Enlarge", ImgproxyName: "enlarge", Short: "el", Control: ControlBool},
					{Key: "extend", Label: "Extend", ImgproxyName: "extend", Short: "ex", Control: ControlExtend, Description: "Extend canvas to target size."},
					{Key: "extend_aspect_ratio", Label: "Extend aspect ratio", ImgproxyName: "extend_aspect_ratio", Short: "exar", Control: ControlExtend, Description: "Extend to match target aspect ratio."},
				},
			},
			{
				ID:    "crop",
				Label: "Crop & gravity",
				Options: []Option{
					{Key: "gravity", Label: "Gravity", ImgproxyName: "gravity", Short: "g", Control: ControlGravity},
					{Key: "crop", Label: "Crop", ImgproxyName: "crop", Short: "c", Control: ControlCrop, Description: "Crop region: width, height, gravity, offsets."},
					{Key: "padding", Label: "Padding", ImgproxyName: "padding", Short: "pd", Control: ControlPadding, Description: "1, 2 (v,h) or 4 (t,r,b,l) ints."},
					{Key: "trim", Label: "Trim", ImgproxyName: "trim", Short: "t", Control: ControlTrim, Description: "Auto-trim borders by color similarity."},
				},
			},
			{
				ID:    "rotation",
				Label: "Rotation & flip",
				Options: []Option{
					{Key: "rotate", Label: "Rotate", ImgproxyName: "rotate", Short: "rot", Control: ControlSelect, Choices: rotations},
					{Key: "auto_rotate", Label: "Auto-rotate (EXIF)", ImgproxyName: "auto_rotate", Short: "ar", Control: ControlBool},
					{Key: "flip", Label: "Flip", ImgproxyName: "flip", Short: "fl", Control: ControlFlip},
				},
			},
			{
				ID:    "filters",
				Label: "Filters",
				Options: []Option{
					{Key: "background", Label: "Background", ImgproxyName: "background", Short: "bg", Control: ControlColor, Description: "Hex color used to flatten transparency."},
					{Key: "blur", Label: "Blur", ImgproxyName: "blur", Short: "bl", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1)},
					{Key: "sharpen", Label: "Sharpen", ImgproxyName: "sharpen", Short: "sh", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1)},
					{Key: "pixelate", Label: "Pixelate", ImgproxyName: "pixelate", Short: "pix", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
				},
			},
			{
				ID:          "watermark",
				Label:       "Watermark",
				Description: "Requires IMGPROXY_WATERMARK_DATA / _PATH / _URL configured server-side.",
				Options: []Option{
					{Key: "watermark", Label: "Watermark", ImgproxyName: "watermark", Short: "wm", Control: ControlWmark},
				},
			},
			{
				ID:    "metadata",
				Label: "Metadata",
				Options: []Option{
					{Key: "strip_metadata", Label: "Strip metadata", ImgproxyName: "strip_metadata", Short: "sm", Control: ControlBool, Default: "true"},
					{Key: "keep_copyright", Label: "Keep copyright", ImgproxyName: "keep_copyright", Short: "kcr", Control: ControlBool},
					{Key: "strip_color_profile", Label: "Strip color profile", ImgproxyName: "strip_color_profile", Short: "scp", Control: ControlBool},
					{Key: "enforce_thumbnail", Label: "Use embedded thumbnail", ImgproxyName: "enforce_thumbnail", Short: "eth", Control: ControlBool, Description: "For HEIC/AVIF — faster but lower quality."},
				},
			},
			{
				ID:    "response",
				Label: "Response",
				Options: []Option{
					{Key: "filename", Label: "Filename", ImgproxyName: "filename", Short: "fn", Control: ControlFilename, Placeholder: "output", Description: "Used in Content-Disposition. Templates: {name}, {i}."},
					{Key: "return_attachment", Label: "Force download (attachment)", ImgproxyName: "return_attachment", Short: "att", Control: ControlBool},
					{Key: "raw", Label: "Raw passthrough", ImgproxyName: "raw", Short: "raw", Control: ControlBool, Description: "Stream source unprocessed."},
					{Key: "cachebuster", Label: "Cachebuster", ImgproxyName: "cachebuster", Short: "cb", Control: ControlText},
					{Key: "expires", Label: "Expires (unix ts)", ImgproxyName: "expires", Short: "exp", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "skip_processing", Label: "Skip processing", ImgproxyName: "skip_processing", Short: "skp", Control: ControlList, Description: "Formats to pass through (e.g. gif, svg)."},
				},
			},
			{
				ID:          "presets",
				Label:       "Presets",
				Description: "Names defined via IMGPROXY_PRESETS. Comma-separated to chain.",
				Options: []Option{
					{Key: "preset", Label: "Preset(s)", ImgproxyName: "preset", Short: "pr", Control: ControlText, Placeholder: "thumb,grayscale"},
				},
			},
			{
				ID:          "security",
				Label:       "Per-request security overrides",
				Description: "Only honored when IMGPROXY_ALLOW_SECURITY_OPTIONS=true on the server.",
				Options: []Option{
					{Key: "max_src_resolution", Label: "Max src resolution (MP)", ImgproxyName: "max_src_resolution", Short: "msr", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1)},
					{Key: "max_src_file_size", Label: "Max src file size (bytes)", ImgproxyName: "max_src_file_size", Short: "msfs", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
					{Key: "max_animation_frames", Label: "Max animation frames", ImgproxyName: "max_animation_frames", Short: "maf", Control: ControlNumber, Min: ptr(1.0), Step: ptr(1.0)},
					{Key: "max_animation_frame_resolution", Label: "Max frame resolution (MP)", ImgproxyName: "max_animation_frame_resolution", Short: "mafr", Control: ControlNumber, Min: ptr(0.0), Step: ptr(0.1)},
					{Key: "max_result_dimension", Label: "Max result dimension", ImgproxyName: "max_result_dimension", Short: "mrd", Control: ControlNumber, Min: ptr(0.0), Step: ptr(1.0)},
				},
			},
		},
	}
}

var formats = []Choice{
	{Value: "", Label: "(keep source)"},
	{Value: "jpeg", Label: "JPEG"},
	{Value: "png", Label: "PNG"},
	{Value: "webp", Label: "WebP"},
	{Value: "avif", Label: "AVIF"},
	{Value: "jxl", Label: "JPEG XL"},
	{Value: "gif", Label: "GIF"},
	{Value: "heic", Label: "HEIC"},
	{Value: "tiff", Label: "TIFF"},
	{Value: "bmp", Label: "BMP"},
	{Value: "ico", Label: "ICO"},
}

var gravities = []Choice{
	{Value: "", Label: "(default)"},
	{Value: "ce", Label: "Center"},
	{Value: "no", Label: "North"},
	{Value: "ea", Label: "East"},
	{Value: "so", Label: "South"},
	{Value: "we", Label: "West"},
	{Value: "noea", Label: "North-east"},
	{Value: "nowe", Label: "North-west"},
	{Value: "soea", Label: "South-east"},
	{Value: "sowe", Label: "South-west"},
	{Value: "sm", Label: "Smart"},
	{Value: "fp", Label: "Focus point"},
}

var resizeTypes = []Choice{
	{Value: "fit", Label: "Fit"},
	{Value: "fill", Label: "Fill"},
	{Value: "fill-down", Label: "Fill down"},
	{Value: "force", Label: "Force"},
	{Value: "auto", Label: "Auto"},
}

var webpPresets = []Choice{
	{Value: "default", Label: "Default"},
	{Value: "picture", Label: "Picture"},
	{Value: "photo", Label: "Photo"},
	{Value: "drawing", Label: "Drawing"},
	{Value: "icon", Label: "Icon"},
	{Value: "text", Label: "Text"},
}

var rotations = []Choice{
	{Value: "", Label: "(none)"},
	{Value: "0", Label: "0°"},
	{Value: "90", Label: "90°"},
	{Value: "180", Label: "180°"},
	{Value: "270", Label: "270°"},
}
