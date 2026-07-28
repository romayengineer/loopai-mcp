package backend

const esc = 0x1B

func StripANSI(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	result := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] != esc {
			result = append(result, data[i])
			i++
			continue
		}

		// We hit an escape sequence.
		i++ // skip ESC

		if i >= len(data) {
			break
		}

		switch {
		case data[i] == '[':
			// CSI sequence: ESC [ ... final byte
			i = skipCSI(data, i+1)

		case data[i] == ']':
			// OSC sequence: ESC ] ... ST (BEL or ESC \)
			i = skipOSC(data, i+1)

		case data[i] == 'P' || data[i] == '_' || data[i] == '^' || data[i] == 'X':
			// DCS, SOS, PM, APC: terminated by ST (ESC \)
			i = skipST(data, i+1)

		case data[i] >= ' ' && data[i] <= '/':
			// Two-character escape: ESC followed by a control char
			i++

		default:
			// Single-character escape: ESC followed by a final byte
			// (7, 8, =, >, D, E, H, M, N, O, Z, \, c, etc.)
			i++
		}
	}

	return result
}

func skipCSI(data []byte, start int) int {
	i := start
	for i < len(data) {
		b := data[i]
		i++
		// Final byte: 0x40-0x7E (@, A-Z, [, \, ], ^, _, `, a-z, {, |, }, ~)
		if b >= 0x40 && b <= 0x7E {
			break
		}
		// Intermediate bytes: 0x20-0x2F (space, !, ", #, $, %, &, ', (, ), *, +, ,, -, ., /)
		// Parameter bytes: 0x30-0x3F (0-9, :, ;, <, =, >, ?)
		// Otherwise invalid — consume and stop.
	}
	return i
}

func skipOSC(data []byte, start int) int {
	i := start
	for i < len(data) {
		b := data[i]
		i++
		if b == '\a' { // BEL terminates OSC
			break
		}
		if b == esc && i < len(data) && data[i] == '\\' { // ESC \ terminates
			i++
			break
		}
	}
	return i
}

func skipST(data []byte, start int) int {
	i := start
	for i < len(data) {
		b := data[i]
		i++
		if b == esc && i < len(data) && data[i] == '\\' {
			i++
			break
		}
	}
	return i
}
