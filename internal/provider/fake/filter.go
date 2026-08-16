package fake

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jamoowen/log-leopard/internal/provider"
)

type filterNode interface {
	match(provider.Entry) bool
}

type logicalFilter struct {
	op          string
	left, right filterNode
}

func (f logicalFilter) match(entry provider.Entry) bool {
	if f.op == "AND" {
		return f.left.match(entry) && f.right.match(entry)
	}
	return f.left.match(entry) || f.right.match(entry)
}

type notFilter struct{ child filterNode }

func (f notFilter) match(entry provider.Entry) bool { return !f.child.match(entry) }

type predicateFilter struct {
	field, op, value string
	timestamp        time.Time
}

func (f predicateFilter) match(entry provider.Entry) bool {
	switch f.field {
	case "resource.type":
		return f.value == "cloud_run_revision"
	case "timestamp":
		switch f.op {
		case ">=":
			return !entry.Timestamp.Before(f.timestamp)
		case "<":
			return entry.Timestamp.Before(f.timestamp)
		case "<=":
			return !entry.Timestamp.After(f.timestamp)
		}
	case "resource.labels.service_name", "jsonPayload.service", "jsonPayload.service_name", "jsonPayload.service.name":
		return entry.Source == f.value
	case "severity":
		return entry.SeverityOriginal == f.value
	case "jsonPayload.requestId", "jsonPayload.request_id", "jsonPayload.request.id", "jsonPayload.httpRequest.requestId":
		return entry.RequestID == f.value
	case "trace", "jsonPayload.trace", `jsonPayload."logging.googleapis.com/trace"`:
		return entry.Trace == f.value
	case "jsonPayload.level":
		level, _ := entry.Structured["level"].(string)
		if f.op == "=" {
			return level == f.value
		}
		return matchesCompiledLevel(level, f.value)
	case "textPayload":
		return entry.MessageSource == "textPayload" && contains(entry.Message, f.value)
	case "jsonPayload.message", "jsonPayload.msg", "jsonPayload.error.message", "jsonPayload.exception.message":
		message := structuredMessage(entry.Structured, strings.TrimPrefix(f.field, "jsonPayload."))
		return contains(message, f.value)
	}
	return false
}

func structuredMessage(payload map[string]any, path string) string {
	parts := strings.Split(path, ".")
	var value any = payload
	for _, part := range parts {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = object[part]
	}
	message, _ := value.(string)
	return message
}

func matchesCompiledLevel(level, pattern string) bool {
	pattern = strings.TrimPrefix(pattern, "(?i)^")
	pattern = strings.TrimSuffix(pattern, "$")
	pattern = strings.TrimPrefix(pattern, "(?:")
	pattern = strings.TrimSuffix(pattern, ")")
	for candidate := range strings.SplitSeq(pattern, "|") {
		if strings.EqualFold(level, candidate) {
			return true
		}
	}
	return false
}

func contains(value, term string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(term))
}

type filterToken struct {
	kind, value string
}

type filterParser struct {
	tokens []filterToken
	pos    int
}

func parseFilter(input string) (filterNode, error) {
	tokens, ok := lexFilter(input)
	if !ok || len(tokens) == 0 {
		return nil, provider.ErrInvalidQuery
	}
	p := filterParser{tokens: tokens}
	filter, ok := p.parseOr()
	if !ok || p.pos != len(tokens) {
		return nil, provider.ErrInvalidQuery
	}
	return filter, nil
}

func (p *filterParser) parseOr() (filterNode, bool) {
	left, ok := p.parseAnd()
	if !ok {
		return nil, false
	}
	for p.take("OR") {
		right, ok := p.parseAnd()
		if !ok {
			return nil, false
		}
		left = logicalFilter{op: "OR", left: left, right: right}
	}
	return left, true
}

func (p *filterParser) parseAnd() (filterNode, bool) {
	left, ok := p.parseUnary()
	if !ok {
		return nil, false
	}
	for p.take("AND") {
		right, ok := p.parseUnary()
		if !ok {
			return nil, false
		}
		left = logicalFilter{op: "AND", left: left, right: right}
	}
	return left, true
}

func (p *filterParser) parseUnary() (filterNode, bool) {
	if p.take("NOT") {
		child, ok := p.parseUnary()
		return notFilter{child: child}, ok
	}
	if p.take("(") {
		child, ok := p.parseOr()
		if !ok || !p.take(")") {
			return nil, false
		}
		return child, true
	}
	return p.parsePredicate()
}

func (p *filterParser) parsePredicate() (filterNode, bool) {
	field, ok := p.next("word")
	if !ok || p.pos >= len(p.tokens) {
		return nil, false
	}
	op := p.tokens[p.pos]
	if op.kind != "operator" {
		return nil, false
	}
	p.pos++
	value, ok := p.next("string")
	if !ok {
		return nil, false
	}
	predicate := predicateFilter{field: field.value, op: op.value, value: value.value}
	if !predicate.supported() {
		return nil, false
	}
	if field.value == "timestamp" {
		predicate.timestamp, _ = time.Parse(time.RFC3339Nano, value.value)
	}
	return predicate, true
}

func (f predicateFilter) supported() bool {
	switch f.field {
	case "resource.type":
		return f.op == "=" && f.value == "cloud_run_revision"
	case "timestamp":
		_, err := time.Parse(time.RFC3339Nano, f.value)
		return err == nil && (f.op == ">=" || f.op == "<" || f.op == "<=")
	case "resource.labels.service_name", "jsonPayload.service", "jsonPayload.service_name", "jsonPayload.service.name", "severity":
		return f.op == "=" && (f.field != "severity" || isCanonicalSeverity(f.value))
	case "jsonPayload.requestId", "jsonPayload.request_id", "jsonPayload.request.id", "jsonPayload.httpRequest.requestId",
		"trace", "jsonPayload.trace", `jsonPayload."logging.googleapis.com/trace"`:
		return f.op == "="
	case "jsonPayload.level":
		return f.op == "=" && isCompiledLevel(f.value) || f.op == "=~" && isCompiledLevelPattern(f.value)
	case "textPayload", "jsonPayload.message", "jsonPayload.msg", "jsonPayload.error.message", "jsonPayload.exception.message":
		return f.op == ":"
	default:
		return false
	}
}

func isCanonicalSeverity(value string) bool {
	return slices.Contains([]string{"DEFAULT", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY"}, value)
}

func isCompiledLevel(value string) bool {
	return slices.Contains([]string{
		"DEFAULT", "default", "DEBUG", "debug", "TRACE", "trace", "INFO", "info", "NOTICE", "notice",
		"WARNING", "warning", "WARN", "warn", "ERROR", "error", "CRITICAL", "critical", "FATAL", "fatal",
		"PANIC", "panic", "ALERT", "alert", "EMERGENCY", "emergency",
	}, value)
}

func isCompiledLevelPattern(pattern string) bool {
	return slices.Contains([]string{
		"(?i)^DEFAULT$", "(?i)^(?:DEBUG|TRACE)$", "(?i)^INFO$", "(?i)^NOTICE$",
		"(?i)^(?:WARNING|WARN)$", "(?i)^ERROR$", "(?i)^(?:CRITICAL|FATAL|PANIC)$",
		"(?i)^ALERT$", "(?i)^EMERGENCY$",
	}, pattern)
}

func (p *filterParser) take(kind string) bool {
	if p.pos >= len(p.tokens) || p.tokens[p.pos].kind != kind {
		return false
	}
	p.pos++
	return true
}

func (p *filterParser) next(kind string) (filterToken, bool) {
	if p.pos >= len(p.tokens) || p.tokens[p.pos].kind != kind {
		return filterToken{}, false
	}
	token := p.tokens[p.pos]
	p.pos++
	return token, true
}

func lexFilter(input string) ([]filterToken, bool) {
	var tokens []filterToken
	for i := 0; i < len(input); {
		if unicode.IsSpace(rune(input[i])) {
			i++
			continue
		}
		switch input[i] {
		case '(', ')':
			tokens = append(tokens, filterToken{kind: string(input[i])})
			i++
			continue
		case '"':
			end := i + 1
			for end < len(input) {
				if input[end] == '\\' {
					end += 2
					continue
				}
				if input[end] == '"' {
					break
				}
				end++
			}
			if end >= len(input) {
				return nil, false
			}
			value, err := strconv.Unquote(input[i : end+1])
			if err != nil {
				return nil, false
			}
			tokens = append(tokens, filterToken{kind: "string", value: value})
			i = end + 1
			continue
		}
		if strings.ContainsRune("=:<>", rune(input[i])) {
			end := i + 1
			if end < len(input) && (input[end] == '=' || input[i] == '=' && input[end] == '~') {
				end++
			}
			tokens = append(tokens, filterToken{kind: "operator", value: input[i:end]})
			i = end
			continue
		}
		end := i
		for end < len(input) && !unicode.IsSpace(rune(input[end])) && !strings.ContainsRune("()=:<>", rune(input[end])) {
			end++
		}
		if end == i {
			return nil, false
		}
		value := input[i:end]
		kind := "word"
		if value == "AND" || value == "OR" || value == "NOT" {
			kind = value
		}
		tokens = append(tokens, filterToken{kind: kind, value: value})
		i = end
	}
	return tokens, true
}
