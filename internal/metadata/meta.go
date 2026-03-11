package metadata

import (
	"bufio"
	"fmt"
	"strings"
	"unicode"

	"github.com/sqlc-dev/sqlc/internal/constants"

	"github.com/sqlc-dev/sqlc/internal/source"
)

type CommentSyntax source.CommentSyntax

type DynamicCheckMode string

const (
	DynamicCheckModeHeuristic  DynamicCheckMode = "heuristic"
	DynamicCheckModeExhaustive DynamicCheckMode = "exhaustive"
)

func ParseDynamicCheckMode(s string) (DynamicCheckMode, error) {
	switch DynamicCheckMode(s) {
	case DynamicCheckModeHeuristic, DynamicCheckModeExhaustive:
		return DynamicCheckMode(s), nil
	default:
		return "", fmt.Errorf("invalid %s mode: %q", constants.QueryFlagSqlcDynamicCheck, s)
	}
}

type Metadata struct {
	Name         string
	Cmd          string
	Comments     []string
	Params       map[string]string
	Flags        map[string]bool
	DynamicCheck DynamicCheckMode

	// RuleSkiplist contains the names of rules to disable vetting for.
	// If the map is empty, but the disable vet flag is specified, then all rules are ignored.
	RuleSkiplist map[string]struct{}

	Filename string
}

type CommentDirectives struct {
	Params       map[string]string
	Flags        map[string]bool
	DynamicCheck DynamicCheckMode
	RuleSkiplist map[string]struct{}
}

type commentDirectiveHandler func(*CommentDirectives, *bufio.Scanner, string) error

var commentDirectiveHandlers = map[string]commentDirectiveHandler{
	constants.QueryFlagParam:            parseParamDirective,
	constants.QueryFlagSqlcDynamicCheck: parseDynamicCheckDirective,
	constants.QueryFlagSqlcVetDisable:   parseVetDisableDirective,
}

const (
	CmdExec       = ":exec"
	CmdExecResult = ":execresult"
	CmdExecRows   = ":execrows"
	CmdExecLastId = ":execlastid"
	CmdMany       = ":many"
	CmdOne        = ":one"
	CmdCopyFrom   = ":copyfrom"
	CmdBatchExec  = ":batchexec"
	CmdBatchMany  = ":batchmany"
	CmdBatchOne   = ":batchone"
)

// A query name must be a valid Go identifier
//
// https://golang.org/ref/spec#Identifiers
func validateQueryName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("invalid query name: %q", name)
	}
	for i, c := range name {
		isLetter := unicode.IsLetter(c) || c == '_'
		isDigit := unicode.IsDigit(c)
		if i == 0 && !isLetter {
			return fmt.Errorf("invalid query name %q", name)
		} else if !(isLetter || isDigit) {
			return fmt.Errorf("invalid query name %q", name)
		}
	}
	return nil
}

func ParseQueryNameAndType(t string, commentStyle CommentSyntax) (string, string, error) {
	for _, line := range strings.Split(t, "\n") {
		var prefix string
		if strings.HasPrefix(line, "--") {
			if !commentStyle.Dash {
				continue
			}
			prefix = "--"
		}
		if strings.HasPrefix(line, "/*") {
			if !commentStyle.SlashStar {
				continue
			}
			prefix = "/*"
		}
		if strings.HasPrefix(line, "#") {
			if !commentStyle.Hash {
				continue
			}
			prefix = "#"
		}
		if prefix == "" {
			continue
		}
		rest := line[len(prefix):]
		if !strings.HasPrefix(strings.TrimSpace(rest), "name") {
			continue
		}
		if !strings.Contains(rest, ":") {
			continue
		}
		if !strings.HasPrefix(rest, " name: ") {
			return "", "", fmt.Errorf("invalid metadata: %s", line)
		}

		part := strings.Split(strings.TrimSpace(line), " ")
		if prefix == "/*" {
			part = part[:len(part)-1] // removes the trailing "*/" element
		}
		if len(part) == 3 {
			return "", "", fmt.Errorf("missing query type [':one', ':many', ':exec', ':execrows', ':execlastid', ':execresult', ':copyfrom', 'batchexec', 'batchmany', 'batchone']: %s", line)
		}
		if len(part) != 4 {
			return "", "", fmt.Errorf("invalid query comment: %s", line)
		}
		queryName := part[2]
		queryType := strings.TrimSpace(part[3])
		switch queryType {
		case CmdOne, CmdMany, CmdExec, CmdExecResult, CmdExecRows, CmdExecLastId, CmdCopyFrom, CmdBatchExec, CmdBatchMany, CmdBatchOne:
		default:
			return "", "", fmt.Errorf("invalid query type: %s", queryType)
		}
		if err := validateQueryName(queryName); err != nil {
			return "", "", err
		}
		return queryName, queryType, nil
	}
	return "", "", nil
}

func newCommentDirectives() CommentDirectives {
	return CommentDirectives{
		Params:       make(map[string]string),
		Flags:        make(map[string]bool),
		RuleSkiplist: make(map[string]struct{}),
	}
}

func parseParamDirective(directives *CommentDirectives, s *bufio.Scanner, _ string) error {
	s.Scan()
	name := s.Text()
	var rest []string
	for s.Scan() {
		paramToken := s.Text()
		rest = append(rest, paramToken)
	}
	directives.Params[name] = strings.Join(rest, " ")
	return nil
}

func parseDynamicCheckDirective(directives *CommentDirectives, s *bufio.Scanner, token string) error {
	directives.Flags[token] = true

	if directives.DynamicCheck != "" {
		return fmt.Errorf("duplicate %s annotation", token)
	}
	if !s.Scan() {
		return fmt.Errorf("missing %s mode", token)
	}

	mode, err := ParseDynamicCheckMode(s.Text())
	if err != nil {
		return err
	}
	directives.DynamicCheck = mode

	if s.Scan() {
		return fmt.Errorf("unexpected extra token %q in %s annotation", s.Text(), token)
	}

	return nil
}

func parseVetDisableDirective(directives *CommentDirectives, s *bufio.Scanner, token string) error {
	directives.Flags[token] = true

	// Vet rules can all be disabled in the same line or split across lines .i.e.
	// /* @sqlc-vet-disable sqlc/db-prepare delete-without-where */
	// is equivalent to:
	// /* @sqlc-vet-disable sqlc/db-prepare */
	// /* @sqlc-vet-disable delete-without-where */
	for s.Scan() {
		directives.RuleSkiplist[s.Text()] = struct{}{}
	}

	return nil
}

// ParseCommentFlags processes the comments provided with queries to determine the metadata params, flags, dynamic check mode and rules to skip.
// All flags in query comments are prefixed with `@`, e.g. @param, @@sqlc-vet-disable.
func ParseCommentFlags(comments []string) (CommentDirectives, error) {
	directives := newCommentDirectives()

	for _, line := range comments {
		s := bufio.NewScanner(strings.NewReader(line))
		s.Split(bufio.ScanWords)

		if !s.Scan() {
			if s.Err() != nil {
				return directives, s.Err()
			}
			continue
		}
		token := s.Text()

		if !strings.HasPrefix(token, "@") {
			if s.Err() != nil {
				return directives, s.Err()
			}
			continue
		}

		if handler, ok := commentDirectiveHandlers[token]; ok {
			if err := handler(&directives, s, token); err != nil {
				return directives, err
			}
		} else {
			directives.Flags[token] = true
		}

		if s.Err() != nil {
			return directives, s.Err()
		}
	}

	return directives, nil
}
