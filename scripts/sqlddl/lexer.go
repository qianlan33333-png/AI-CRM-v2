package sqlddl

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenWord tokenKind = iota
	tokenQuotedIdentifier
	tokenLiteral
	tokenSymbol
)

type token struct {
	kind tokenKind
	raw  string
}

func (t token) keyword(word string) bool {
	return t.kind == tokenWord && strings.EqualFold(t.raw, word)
}

func (t token) identifier() (string, bool) {
	switch t.kind {
	case tokenWord:
		return strings.ToLower(t.raw), true
	case tokenQuotedIdentifier:
		return t.raw, true
	default:
		return "", false
	}
}

func lex(source string) ([]token, error) {
	var tokens []token
	for offset := 0; offset < len(source); {
		r, size := utf8.DecodeRuneInString(source[offset:])
		if unicode.IsSpace(r) {
			offset += size
			continue
		}
		if strings.HasPrefix(source[offset:], "--") {
			newline := strings.IndexByte(source[offset+2:], '\n')
			if newline < 0 {
				break
			}
			offset += newline + 3
			continue
		}
		if strings.HasPrefix(source[offset:], "/*") {
			end, err := blockCommentEnd(source, offset)
			if err != nil {
				return nil, err
			}
			offset = end
			continue
		}
		switch source[offset] {
		case '\'':
			end, err := quotedEnd(source, offset, '\'')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenLiteral, raw: source[offset:end]})
			offset = end
			continue
		case '"':
			end, err := quotedEnd(source, offset, '"')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenQuotedIdentifier, raw: source[offset:end]})
			offset = end
			continue
		case '$':
			if delimiter, ok := dollarDelimiter(source[offset:]); ok {
				end := strings.Index(source[offset+len(delimiter):], delimiter)
				if end < 0 {
					return nil, fmt.Errorf("sqlddl: unterminated dollar-quoted literal at byte %d", offset)
				}
				end += offset + 2*len(delimiter)
				tokens = append(tokens, token{kind: tokenLiteral, raw: source[offset:end]})
				offset = end
				continue
			}
		}
		if isWordStart(r) || unicode.IsDigit(r) {
			end := offset + size
			for end < len(source) {
				next, nextSize := utf8.DecodeRuneInString(source[end:])
				if !isWordPart(next) {
					break
				}
				end += nextSize
			}
			tokens = append(tokens, token{kind: tokenWord, raw: source[offset:end]})
			offset = end
			continue
		}
		end := offset + size
		for _, operator := range []string{"->>", "#>>", "::", "<=", ">=", "<>", "!=", "||", "&&", "->", "#>"} {
			if strings.HasPrefix(source[offset:], operator) {
				end = offset + len(operator)
				break
			}
		}
		tokens = append(tokens, token{kind: tokenSymbol, raw: source[offset:end]})
		offset = end
	}
	return tokens, nil
}

func blockCommentEnd(source string, offset int) (int, error) {
	depth := 1
	for cursor := offset + 2; cursor < len(source); {
		switch {
		case strings.HasPrefix(source[cursor:], "/*"):
			depth++
			cursor += 2
		case strings.HasPrefix(source[cursor:], "*/"):
			depth--
			cursor += 2
			if depth == 0 {
				return cursor, nil
			}
		default:
			_, size := utf8.DecodeRuneInString(source[cursor:])
			cursor += size
		}
	}
	return 0, fmt.Errorf("sqlddl: unterminated block comment at byte %d", offset)
}

func quotedEnd(source string, offset int, quote byte) (int, error) {
	for cursor := offset + 1; cursor < len(source); cursor++ {
		if quote == '\'' && source[cursor] == '\\' && cursor+1 < len(source) {
			cursor++
			continue
		}
		if source[cursor] != quote {
			continue
		}
		if cursor+1 < len(source) && source[cursor+1] == quote {
			cursor++
			continue
		}
		return cursor + 1, nil
	}
	return 0, fmt.Errorf("sqlddl: unterminated quoted value at byte %d", offset)
}

func dollarDelimiter(source string) (string, bool) {
	if len(source) < 2 || source[0] != '$' {
		return "", false
	}
	end := strings.IndexByte(source[1:], '$')
	if end < 0 {
		return "", false
	}
	end++
	for _, r := range source[1:end] {
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return "", false
		}
	}
	return source[:end+1], true
}

func isWordStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isWordPart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
