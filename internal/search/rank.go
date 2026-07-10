package search

import (
	"strconv"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

func resultFromChromem(r chromem.Result) Result {
	return Result{
		ID:        r.ID,
		ProjectID: r.Metadata["project_id"],
		Path:      r.Metadata["path"],
		Language:  r.Metadata["language"],
		Symbol:    r.Metadata["symbol"],
		Signature: r.Metadata["signature"],
		Content:   r.Content,
		StartLine: atoi(r.Metadata["start_line"]),
		EndLine:   atoi(r.Metadata["end_line"]),
		Score:     r.Similarity,
	}
}

func boostScore(res Result, query string, base float32) float32 {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(res.Symbol), q) {
		return base + 0.1
	}
	if strings.Contains(strings.ToLower(res.Path), q) {
		return base + 0.05
	}
	return base
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
