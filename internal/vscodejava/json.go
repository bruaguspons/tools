package vscodejava

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// decodeSettings parses raw settings.json bytes into an order-preserving
// representation. An absent or blank file is treated as an empty object.
func decodeSettings(data []byte) ([]string, map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []string{}, map[string]json.RawMessage{}, nil
	}
	return decodeOrderedObject(data, trimmed)
}

// decodeOrderedObject strictly decodes a JSON object, recording the
// original top-level key order alongside each key's raw (unparsed) value.
// It rejects anything that isn't a top-level object, including any
// trailing content after the closing brace.
func decodeOrderedObject(original, trimmed []byte) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(trimmed))

	tok, err := dec.Token()
	if err != nil {
		return nil, nil, describeSyntaxError(original, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, nil, fmt.Errorf("top-level JSON value must be an object, found %v", tok)
	}

	order := make([]string, 0)
	values := make(map[string]json.RawMessage)

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, describeSyntaxError(original, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected a string object key, found %v", keyTok)
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, describeSyntaxError(original, err)
		}

		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = raw
	}

	if _, err := dec.Token(); err != nil { // consume closing '}'
		return nil, nil, describeSyntaxError(original, err)
	}
	if dec.More() {
		return nil, nil, errors.New("unexpected content after the top-level JSON object")
	}

	return order, values, nil
}

// describeSyntaxError wraps a JSON decoding error with a line:column
// location and, when detectable, an explicit hint that the file looks
// like JSONC (comments or a trailing comma) rather than strict JSON.
func describeSyntaxError(data []byte, err error) error {
	var synErr *json.SyntaxError
	if errors.As(err, &synErr) {
		line, col := lineCol(data, synErr.Offset)
		hint := ""
		switch {
		case looksLikeComment(data):
			hint = "; settings.json appears to contain // or /* comments — this tool requires strict JSON (JSONC is not supported): remove all comments and trailing commas"
		case looksLikeTrailingComma(data):
			hint = "; settings.json appears to contain a trailing comma — this tool requires strict JSON: remove trailing commas"
		}
		return fmt.Errorf("invalid JSON at line %d, column %d: %v%s", line, col, err, hint)
	}
	return fmt.Errorf("invalid JSON: %w", err)
}

// lineCol converts a byte offset into a 1-based line and column.
func lineCol(data []byte, offset int64) (line, col int) {
	if offset < 0 || offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line, col = 1, 1
	for i := int64(0); i < offset; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func looksLikeComment(data []byte) bool {
	return bytes.Contains(data, []byte("//")) || bytes.Contains(data, []byte("/*"))
}

func looksLikeTrailingComma(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] != ',' {
			continue
		}
		j := i + 1
		for j < len(data) && isJSONSpace(data[j]) {
			j++
		}
		if j < len(data) && (data[j] == '}' || data[j] == ']') {
			return true
		}
	}
	return false
}

func isJSONSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

// detectIndent inspects existing settings.json content for the
// indentation used on its first indented line, defaulting to 4 spaces
// (VS Code's own default) when none can be detected (absent, blank, or
// single-line "{}" file).
func detectIndent(data []byte) string {
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimLeft(line, " ")
		n := len(line) - len(trimmed)
		if n > 0 && len(trimmed) > 0 && trimmed[0] == '"' {
			return string(line[:n])
		}
	}
	return defaultIndent
}

// render emits a JSON object with keys in the given order, one per line,
// indented with indentUnit. Each value is reformatted through json.Indent
// so nested structures line up with the chosen indent, but is otherwise
// byte-for-byte whatever was already stored for that key (untouched
// values are never semantically altered, only re-indented).
func render(order []string, values map[string]json.RawMessage, indentUnit string) ([]byte, error) {
	var buf bytes.Buffer

	if len(order) == 0 {
		buf.WriteString("{}\n")
		return buf.Bytes(), nil
	}

	buf.WriteString("{\n")
	for i, key := range order {
		raw, ok := values[key]
		if !ok {
			return nil, fmt.Errorf("internal error: key %q listed in order but missing from values", key)
		}

		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("marshal key %q: %w", key, err)
		}

		var valBuf bytes.Buffer
		if err := json.Indent(&valBuf, raw, indentUnit, indentUnit); err != nil {
			return nil, fmt.Errorf("indent value for key %q: %w", key, err)
		}

		buf.WriteString(indentUnit)
		buf.Write(keyJSON)
		buf.WriteString(": ")
		buf.Write(valBuf.Bytes())
		if i < len(order)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")

	return buf.Bytes(), nil
}
