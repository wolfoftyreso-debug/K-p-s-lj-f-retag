package httpapi

import "unicode/utf8"

// validJSONEncoding prevents encoding/json's replacement of malformed Unicode
// from silently changing a command or merging distinct idempotency requests.
// JSON grammar, object shape and concrete types are validated by decodeCommand.
func validJSONEncoding(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		code, ok := hexCode(data, i+1)
		if !ok {
			return false
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code < 0xd800 || code > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		low, ok := hexCode(data, i+3)
		if !ok || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return !inString
}

func hexCode(data []byte, start int) (uint16, bool) {
	if start+4 > len(data) {
		return 0, false
	}
	var code uint16
	for _, b := range data[start : start+4] {
		code <<= 4
		switch {
		case b >= '0' && b <= '9':
			code |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			code |= uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			code |= uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return code, true
}
