package source

import (
	"fmt"
	"strings"
)

type DynamicArmKind string

const (
	DynamicArmKindIf   DynamicArmKind = "if"
	DynamicArmKindElif DynamicArmKind = "elif"
	DynamicArmKindElse DynamicArmKind = "else"

	dynamicDirectiveEndif = "endif"
)

type DynamicQuery struct {
	Parts []DynamicPart
}

type DynamicPart struct {
	Text string
	If   *DynamicIfBlock
}

type DynamicIfBlock struct {
	Arms []DynamicIfArm
}

type DynamicIfArm struct {
	Kind      DynamicArmKind
	Condition string
	Parts     []DynamicPart
}

type dynamicFrame struct {
	block       *DynamicIfBlock
	parentParts *[]DynamicPart
	sawElse     bool
}

func ParseDynamicQuery(sql string) (DynamicQuery, error) {
	query := DynamicQuery{}
	currentParts := &query.Parts
	textStart := 0
	var stack []dynamicFrame

	for i := 0; i < len(sql); {
		switch {
		case strings.HasPrefix(sql[i:], "[["):
			close := strings.Index(sql[i+2:], "]]")
			if close < 0 {
				return query, fmt.Errorf("unterminated dynamic control directive")
			}

			appendDynamicText(currentParts, sql[textStart:i])

			body := sql[i+2 : i+2+close]
			directive, condition, err := parseDynamicDirective(body)
			if err != nil {
				return query, err
			}

			switch directive {
			case string(DynamicArmKindIf):
				block := &DynamicIfBlock{
					Arms: []DynamicIfArm{{
						Kind:      DynamicArmKindIf,
						Condition: condition,
					}},
				}
				*currentParts = append(*currentParts, DynamicPart{If: block})
				stack = append(stack, dynamicFrame{
					block:       block,
					parentParts: currentParts,
				})
				currentParts = &block.Arms[0].Parts

			case string(DynamicArmKindElif):
				if len(stack) == 0 {
					return query, fmt.Errorf("unexpected [[elif]] without matching [[if]]")
				}
				frame := &stack[len(stack)-1]
				if frame.sawElse {
					return query, fmt.Errorf("unexpected [[elif]] after [[else]]")
				}

				frame.block.Arms = append(frame.block.Arms, DynamicIfArm{
					Kind:      DynamicArmKindElif,
					Condition: condition,
				})
				currentParts = &frame.block.Arms[len(frame.block.Arms)-1].Parts

			case string(DynamicArmKindElse):
				if len(stack) == 0 {
					return query, fmt.Errorf("unexpected [[else]] without matching [[if]]")
				}
				frame := &stack[len(stack)-1]
				if frame.sawElse {
					return query, fmt.Errorf("duplicate [[else]] in dynamic control block")
				}

				frame.sawElse = true
				frame.block.Arms = append(frame.block.Arms, DynamicIfArm{Kind: DynamicArmKindElse})
				currentParts = &frame.block.Arms[len(frame.block.Arms)-1].Parts

			case dynamicDirectiveEndif:
				if len(stack) == 0 {
					return query, fmt.Errorf("unexpected [[endif]] without matching [[if]]")
				}
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				currentParts = frame.parentParts

			default:
				return query, fmt.Errorf("unknown dynamic control directive: %q", directive)
			}

			i += close + 4
			textStart = i

		case strings.HasPrefix(sql[i:], "--"):
			i = consumeLineComment(sql, i+2)

		case sql[i] == '#':
			i = consumeLineComment(sql, i+1)

		case strings.HasPrefix(sql[i:], "/*"):
			next, err := consumeBlockComment(sql, i+2)
			if err != nil {
				return query, err
			}
			i = next

		case sql[i] == '\'':
			next, err := consumeQuoted(sql, i, '\'')
			if err != nil {
				return query, err
			}
			i = next

		case sql[i] == '"':
			next, err := consumeQuoted(sql, i, '"')
			if err != nil {
				return query, err
			}
			i = next

		case sql[i] == '`':
			next, err := consumeQuoted(sql, i, '`')
			if err != nil {
				return query, err
			}
			i = next

		case sql[i] == '$':
			tag, ok := scanDollarQuoteTag(sql, i)
			if !ok {
				i++
				continue
			}
			next, err := consumeDollarQuote(sql, i, tag)
			if err != nil {
				return query, err
			}
			i = next

		default:
			i++
		}
	}

	appendDynamicText(currentParts, sql[textStart:])

	if len(stack) > 0 {
		return query, fmt.Errorf("missing [[endif]] for dynamic control block")
	}

	return query, nil
}

func appendDynamicText(parts *[]DynamicPart, text string) {
	if text == "" {
		return
	}
	if len(*parts) > 0 {
		last := &(*parts)[len(*parts)-1]
		if last.If == nil {
			last.Text += text
			return
		}
	}
	*parts = append(*parts, DynamicPart{Text: text})
}

func parseDynamicDirective(body string) (string, string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty dynamic control directive")
	}

	name := trimmed
	rest := ""
	if idx := strings.IndexAny(trimmed, " \t\r\n"); idx >= 0 {
		name = trimmed[:idx]
		rest = strings.TrimSpace(trimmed[idx:])
	}

	switch name {
	case string(DynamicArmKindIf), string(DynamicArmKindElif):
		if rest == "" {
			return "", "", fmt.Errorf("missing condition for [[%s]]", name)
		}
		return name, rest, nil

	case string(DynamicArmKindElse), dynamicDirectiveEndif:
		if rest != "" {
			return "", "", fmt.Errorf("unexpected extra content in [[%s]]", name)
		}
		return name, "", nil

	default:
		return "", "", fmt.Errorf("unknown dynamic control directive: %q", name)
	}
}

func consumeLineComment(sql string, i int) int {
	for i < len(sql) && sql[i] != '\n' {
		i++
	}
	return i
}

func consumeBlockComment(sql string, i int) (int, error) {
	depth := 1
	for i < len(sql)-1 {
		switch {
		case strings.HasPrefix(sql[i:], "/*"):
			depth++
			i += 2
		case strings.HasPrefix(sql[i:], "*/"):
			depth--
			i += 2
			if depth == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated block comment while scanning dynamic control")
}

func consumeQuoted(sql string, i int, delim byte) (int, error) {
	i++
	for i < len(sql) {
		switch {
		case sql[i] == '\\' && i+1 < len(sql):
			i += 2
		case sql[i] == delim:
			if i+1 < len(sql) && sql[i+1] == delim {
				i += 2
				continue
			}
			return i + 1, nil
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated quoted string while scanning dynamic control")
}

func scanDollarQuoteTag(sql string, i int) (string, bool) {
	if sql[i] != '$' {
		return "", false
	}

	j := i + 1
	for j < len(sql) && sql[j] != '$' {
		if !isDollarQuoteTagChar(sql[j]) {
			return "", false
		}
		j++
	}
	if j >= len(sql) || sql[j] != '$' {
		return "", false
	}
	if j > i+1 && isASCIIDigit(sql[i+1]) {
		return "", false
	}
	return sql[i : j+1], true
}

func consumeDollarQuote(sql string, i int, tag string) (int, error) {
	start := i + len(tag)
	close := strings.Index(sql[start:], tag)
	if close < 0 {
		return 0, fmt.Errorf("unterminated dollar-quoted string while scanning dynamic control")
	}
	return start + close + len(tag), nil
}

func isDollarQuoteTagChar(b byte) bool {
	return b == '_' || isASCIIAlpha(b) || isASCIIDigit(b)
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
