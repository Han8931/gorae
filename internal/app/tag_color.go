package app

import "hash/fnv"

// tagPalette is a curated set of bright, well-separated hues for the colored
// circles shown beside tags in the tree pane. A tag is mapped to one of these
// by hashing its name, so a given tag always gets the same circle across
// repaints and sessions — the colors are decorative wayfinding, not semantic.
var tagPalette = []string{
	"#e06c75", // red
	"#e59572", // orange
	"#e5c07b", // yellow
	"#98c379", // green
	"#56b6c2", // cyan
	"#61afef", // blue
	"#c678dd", // purple
	"#d19a66", // amber
	"#7ec9a8", // teal
	"#f28fad", // pink
	"#b48ead", // mauve
	"#89ddff", // sky
}

// tagCircleColor returns the stable circle color for tag. Hierarchical tags are
// colored by their top-level segment (the text before the first "/") so that a
// family of tags such as "ml/nlp" and "ml/cv" shares a hue, reading as a group
// in the tree. Returns "" only when the palette is empty.
func tagCircleColor(tag string) string {
	if len(tagPalette) == 0 {
		return ""
	}
	root := tag
	for i := 0; i < len(tag); i++ {
		if tag[i] == '/' {
			root = tag[:i]
			break
		}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(root))
	return tagPalette[int(h.Sum32())%len(tagPalette)]
}
