package sftp

import (
	"strings"
	"unicode"
)

type Token struct {
	Value  string
	Raw    string
	Start  int
	End    int
	Quoted bool
}

type ParsedLine struct {
	Raw                 string
	Tokens              []Token
	CursorToken         int
	CursorOffsetInToken int
	EndsWithSpace       bool
	DanglingEscape      bool
	UnterminatedQuote   rune
}

func (p ParsedLine) Incomplete() bool {
	return p.DanglingEscape || p.UnterminatedQuote != 0
}

func (p ParsedLine) Values() []string {
	values := make([]string, len(p.Tokens))
	for i, token := range p.Tokens {
		values[i] = token.Value
	}
	return values
}

func ParseLine(raw string, cursor int) ParsedLine {
	runes := []rune(raw)
	if cursor < 0 || cursor > len(runes) {
		cursor = len(runes)
	}

	parsed := ParsedLine{
		Raw:         raw,
		CursorToken: -1,
	}

	if len(runes) > 0 && unicode.IsSpace(runes[len(runes)-1]) {
		parsed.EndsWithSpace = true
	}

	const (
		stateNormal = iota
		stateSingle
		stateDouble
	)

	state := stateNormal
	tokenStart := -1
	var rawBuf []rune
	var valueBuf []rune
	tokenQuoted := false

	startToken := func(pos int) {
		if tokenStart >= 0 {
			return
		}
		tokenStart = pos
		rawBuf = rawBuf[:0]
		valueBuf = valueBuf[:0]
		tokenQuoted = false
	}

	finishToken := func(end int) {
		if tokenStart < 0 {
			return
		}
		parsed.Tokens = append(parsed.Tokens, Token{
			Value:  string(valueBuf),
			Raw:    string(rawBuf),
			Start:  tokenStart,
			End:    end,
			Quoted: tokenQuoted,
		})
		tokenStart = -1
		rawBuf = nil
		valueBuf = nil
		tokenQuoted = false
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		switch state {
		case stateNormal:
			if unicode.IsSpace(r) {
				finishToken(i)
				continue
			}

			switch r {
			case '\'':
				startToken(i)
				rawBuf = append(rawBuf, r)
				tokenQuoted = true
				state = stateSingle
			case '"':
				startToken(i)
				rawBuf = append(rawBuf, r)
				tokenQuoted = true
				state = stateDouble
			case '\\':
				startToken(i)
				rawBuf = append(rawBuf, r)
				if i+1 >= len(runes) {
					parsed.DanglingEscape = true
					valueBuf = append(valueBuf, r)
					continue
				}

				next := runes[i+1]
				if isEscapableInNormal(next) {
					rawBuf = append(rawBuf, next)
					valueBuf = append(valueBuf, next)
					i++
					continue
				}

				valueBuf = append(valueBuf, r)
			default:
				startToken(i)
				rawBuf = append(rawBuf, r)
				valueBuf = append(valueBuf, r)
			}

		case stateSingle:
			rawBuf = append(rawBuf, r)
			if r == '\'' {
				state = stateNormal
				continue
			}
			valueBuf = append(valueBuf, r)

		case stateDouble:
			switch r {
			case '"':
				rawBuf = append(rawBuf, r)
				tokenQuoted = true
				state = stateNormal
			case '\\':
				rawBuf = append(rawBuf, r)
				if i+1 >= len(runes) {
					parsed.DanglingEscape = true
					valueBuf = append(valueBuf, r)
					continue
				}

				next := runes[i+1]
				if isEscapableInDouble(next) {
					rawBuf = append(rawBuf, next)
					valueBuf = append(valueBuf, next)
					i++
					continue
				}

				valueBuf = append(valueBuf, r)
			default:
				rawBuf = append(rawBuf, r)
				valueBuf = append(valueBuf, r)
			}
		}
	}

	switch state {
	case stateSingle:
		parsed.UnterminatedQuote = '\''
	case stateDouble:
		parsed.UnterminatedQuote = '"'
	}

	finishToken(len(runes))
	locateCursor(&parsed, cursor)
	return parsed
}

func locateCursor(parsed *ParsedLine, cursor int) {
	for i, token := range parsed.Tokens {
		if cursor < token.Start {
			parsed.CursorToken = i
			parsed.CursorOffsetInToken = 0
			return
		}
		if cursor >= token.Start && cursor <= token.End {
			parsed.CursorToken = i
			parsed.CursorOffsetInToken = cursor - token.Start
			return
		}
	}

	parsed.CursorToken = len(parsed.Tokens)
	if len(parsed.Tokens) == 0 {
		parsed.CursorOffsetInToken = 0
		return
	}

	last := parsed.Tokens[len(parsed.Tokens)-1]
	if cursor < last.End {
		parsed.CursorOffsetInToken = 0
		return
	}
	parsed.CursorOffsetInToken = cursor - last.End
}

func isEscapableInNormal(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(`"'\\`, r)
}

func isEscapableInDouble(r rune) bool {
	return strings.ContainsRune(`"\\`, r)
}
