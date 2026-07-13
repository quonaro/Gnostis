package cascade

import (
	"fmt"
	"unicode/utf8"

	"github.com/quonaro/gnostis/internal/chat_providers"
)

// ExtractDialogue parses a decrypted CortexTrajectory protobuf message and
// returns the user/assistant turns it contains.
//
// It uses the known wire tags from the public reverse-engineered schema:
//   - field 2 (repeated message): trajectory steps
//   - field 19 (message): user_input
//   - field 20 (message): planner_response (assistant)
func ExtractDialogue(data []byte) []chat_providers.Turn {
	var turns []chat_providers.Turn
	pos := 0
	for pos < len(data) {
		tag, nextPos, err := readVarint(data, pos)
		if err != nil {
			break
		}
		pos = nextPos
		fieldNum := tag >> 3
		wireType := tag & 0x07

		if fieldNum == 2 && wireType == 2 {
			length, nextPos, err := readVarint(data, pos)
			if err != nil {
				break
			}
			pos = nextPos
			end := pos + int(length)
			if end > len(data) {
				break
			}
			stepData := data[pos:end]
			pos = end

			user, assistant := extractStep(stepData)
			if user != "" {
				turns = append(turns, chat_providers.Turn{Role: "user", Content: user})
			}
			if assistant != "" {
				turns = append(turns, chat_providers.Turn{Role: "assistant", Content: assistant})
			}
		} else if wireType == 2 {
			length, nextPos, err := readVarint(data, pos)
			if err != nil {
				break
			}
			pos = nextPos + int(length)
		} else if wireType == 0 {
			_, nextPos, err := readVarint(data, pos)
			if err != nil {
				break
			}
			pos = nextPos
		} else if wireType == 1 {
			pos += 8
		} else if wireType == 5 {
			pos += 4
		} else {
			break
		}
	}
	return turns
}

func extractStep(data []byte) (string, string) {
	var userParts, assistantParts []string
	pos := 0
	for pos < len(data) {
		tag, nextPos, err := readVarint(data, pos)
		if err != nil {
			break
		}
		pos = nextPos
		fieldNum := tag >> 3
		wireType := tag & 0x07

		if wireType == 2 {
			length, nextPos, err := readVarint(data, pos)
			if err != nil {
				break
			}
			pos = nextPos
			end := pos + int(length)
			if end > len(data) {
				break
			}
			payload := data[pos:end]
			pos = end

			switch fieldNum {
			case 19:
				userParts = append(userParts, extractStrings(payload)...)
			case 20:
				assistantParts = append(assistantParts, extractStrings(payload)...)
			}
		} else if wireType == 0 {
			_, nextPos, err := readVarint(data, pos)
			if err != nil {
				break
			}
			pos = nextPos
		} else if wireType == 1 {
			pos += 8
		} else if wireType == 5 {
			pos += 4
		} else {
			break
		}
	}
	return join(userParts), join(assistantParts)
}

func join(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	var out string
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func readVarint(data []byte, pos int) (uint64, int, error) {
	var value uint64
	var shift uint
	for {
		if pos >= len(data) {
			return 0, 0, fmt.Errorf("unexpected end of varint")
		}
		b := data[pos]
		pos++
		value |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return value, pos, nil
		}
		shift += 7
		if shift > 63 {
			return 0, 0, fmt.Errorf("varint overflow")
		}
	}
}

// extractStrings returns printable UTF-8 strings found in the data.
// This is the Go equivalent of the Python strings-style scanner.
func extractStrings(data []byte) []string {
	const minLen = 4
	var strings []string
	start := -1
	for i := 0; i < len(data); i++ {
		if isPrintable(data[i]) {
			if start == -1 {
				start = i
			}
		} else {
			if start != -1 && i-start >= minLen {
				if s := tryDecode(data[start:i]); s != "" {
					strings = append(strings, s)
				}
			}
			start = -1
		}
	}
	if start != -1 && len(data)-start >= minLen {
		if s := tryDecode(data[start:]); s != "" {
			strings = append(strings, s)
		}
	}
	return strings
}

func isPrintable(b byte) bool {
	return b == '\t' || b == '\n' || b == '\r' || (b >= ' ' && b <= '~')
}

func tryDecode(data []byte) string {
	if !utf8.Valid(data) {
		return ""
	}
	return string(data)
}

// ExtractStrings is exported for callers that only need the raw string list.
func ExtractStrings(data []byte) []string {
	return extractStrings(data)
}
