package grammar

import (
	"sort"
	"strings"
)

// chunkBytes bounds one request. UTF-8 bytes are a conservative bound for the characters
// LanguageTool counts.
const chunkBytes = 4000

// chunk is one prepared-space range sent as a single request.
type chunk struct {
	start int
	end   int
}

// chunkPrepared splits the prepared prose at blank-line boundaries and packs the paragraphs
// greedily. A single paragraph above the limit stays one request: sentence splitting would trade a
// rare timeout for a silent loss of context.
func chunkPrepared(text string, limit int) []chunk {
	paragraphs := paragraphRanges(text)
	if len(paragraphs) == 0 {
		return nil
	}
	if limit <= 0 {
		return []chunk{{start: 0, end: len(text)}}
	}
	chunks := make([]chunk, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		size := paragraph.end - paragraph.start
		if len(chunks) > 0 {
			last := chunks[len(chunks)-1]
			if last.end-last.start+size+1 <= limit {
				chunks[len(chunks)-1].end = paragraph.end
				continue
			}
		}
		chunks = append(chunks, paragraph)
	}
	return chunks
}

// paragraphRanges returns the prepared-space ranges of the non-blank paragraph bodies. Blank lines
// separate chunks and are never sent on their own; they cannot carry a finding.
func paragraphRanges(text string) []chunk {
	ranges := make([]chunk, 0)
	start := -1
	end := 0
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.TrimSpace(line) == "" {
			if start >= 0 {
				ranges = append(ranges, chunk{start: start, end: end})
				start = -1
			}
		} else {
			if start < 0 {
				start = offset
			}
			end = offset + len(strings.TrimRight(line, "\r\n"))
		}
		offset += len(line)
	}
	if start >= 0 {
		ranges = append(ranges, chunk{start: start, end: end})
	}
	return ranges
}

// mergeFindings orders findings independently of the chunk count and collapses a finding that two
// chunks reported at the same body position.
func mergeFindings(findings []Finding) []Finding {
	sort.SliceStable(findings, func(left, right int) bool {
		a, b := findings[left], findings[right]
		if a.Offset != b.Offset {
			return a.Offset < b.Offset
		}
		if a.Length != b.Length {
			return a.Length < b.Length
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
	merged := findings[:0]
	for _, finding := range findings {
		if len(merged) > 0 {
			last := merged[len(merged)-1]
			if last.Offset == finding.Offset && last.Length == finding.Length &&
				last.Rule == finding.Rule && last.Message == finding.Message {
				continue
			}
		}
		merged = append(merged, finding)
	}
	return merged
}
