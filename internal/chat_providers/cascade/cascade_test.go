package cascade

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"os"
	"testing"

	"github.com/quonaro/gnostis/internal/chat_providers"
)

func encryptForTest(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, ciphertext...), nil
}

func TestDecrypt(t *testing.T) {
	plaintext := []byte("hello cascade world")
	data, err := encryptForTest(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := Decrypt(data)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("decrypt = %q, want %q", got, plaintext)
	}
}

func TestDecryptShortFile(t *testing.T) {
	if _, err := Decrypt([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestExtractDialogue(t *testing.T) {
	// Build a synthetic protobuf message with one step containing user and assistant fields.
	userPayload := []byte("hello world")
	assistantPayload := []byte("hi there")

	userField := encodeLengthDelimited(19, userPayload)
	assistantField := encodeLengthDelimited(20, assistantPayload)
	stepPayload := append(userField, assistantField...)
	stepField := encodeLengthDelimited(2, stepPayload)

	turns := ExtractDialogue(stepField)
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Role != "user" || !stringsContains(turns[0].Content, "hello world") {
		t.Errorf("user turn = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || !stringsContains(turns[1].Content, "hi there") {
		t.Errorf("assistant turn = %+v", turns[1])
	}
}

func TestExtractStrings(t *testing.T) {
	data := []byte("abcd\x00abcd\x09jkl")
	got := ExtractStrings(data)
	want := []string{"abcd", "abcd\tjkl"}
	if len(got) != len(want) {
		t.Fatalf("ExtractStrings(%q) = %v, want %v", data, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ExtractStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExportSession(t *testing.T) {
	dir := t.TempDir()
	plaintext := buildSimpleDialogue()

	exp := chat_providers.Exporter{MinUserMessageLength: 1}
	path, err := exp.ExportSession(NewProvider(), "/tmp/session.pb", dir, plaintext)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported: %v", err)
	}
	if !bytes.Contains(data, []byte("### User")) {
		t.Errorf("exported markdown missing User section")
	}
	if !bytes.Contains(data, []byte("### Assistant")) {
		t.Errorf("exported markdown missing Assistant section")
	}
}

func buildSimpleDialogue() []byte {
	userPayload := []byte("user question")
	assistantPayload := []byte("assistant answer")
	stepPayload := append(encodeLengthDelimited(19, userPayload), encodeLengthDelimited(20, assistantPayload)...)
	return encodeLengthDelimited(2, stepPayload)
}

func encodeLengthDelimited(fieldNum int, payload []byte) []byte {
	tag := (fieldNum << 3) | 2
	var b bytes.Buffer
	b.Write(encodeVarint(uint64(tag)))
	b.Write(encodeVarint(uint64(len(payload))))
	b.Write(payload)
	return b.Bytes()
}

func encodeVarint(v uint64) []byte {
	var b []byte
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	b = append(b, byte(v))
	return b
}

func stringsContains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestReadVarint(t *testing.T) {
	cases := []struct {
		data []byte
		want uint64
	}{
		{encodeVarint(1), 1},
		{encodeVarint(150), 150},
		{encodeVarint(1 << 35), 1 << 35},
	}
	for _, tc := range cases {
		got, pos, err := readVarint(tc.data, 0)
		if err != nil {
			t.Errorf("readVarint(%v): %v", tc.data, err)
			continue
		}
		if got != tc.want || pos != len(tc.data) {
			t.Errorf("readVarint(%v) = (%d, %d), want (%d, %d)", tc.data, got, pos, tc.want, len(tc.data))
		}
	}
}

func TestReadVarintTruncated(t *testing.T) {
	data := []byte{0x80, 0x80}
	if _, _, err := readVarint(data, 0); err == nil {
		t.Fatal("expected error for truncated varint")
	}
}
