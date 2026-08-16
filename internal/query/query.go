package query

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	MaxWindow     = 7 * 24 * time.Hour
	MaxPredicates = 50
	// MaxFilterBytes leaves headroom below Cloud Logging's 20,000-character filter limit.
	MaxFilterBytes = 19_000
)

type FieldPredicate struct {
	Path     string `json:"path" maxLength:"268" doc:"Dot-separated structured payload path, optionally prefixed with jsonPayload. The normalized path may contain up to 256 characters. Each segment must start with a letter or underscore and contain only letters, digits, and underscores."`
	Operator string `json:"operator" enum:"equals,contains,exists,gt,lt" doc:"Exact structured-field comparison operator."`
	Value    any    `json:"value" doc:"Comparison value: string, number, or boolean for equals; string for contains; boolean for exists; number for gt or lt."`
}

type CompileInput struct {
	Text         string
	Sources      []string
	Severities   []string
	Start        time.Time
	End          time.Time
	NativeFilter string
	Predicates   []FieldPredicate
}

func Compile(in CompileInput) (string, error) {
	if in.Start.IsZero() || in.End.IsZero() || !in.Start.Before(in.End) {
		return "", errors.New("start and end must define a positive absolute window")
	}
	if in.End.Sub(in.Start) > MaxWindow {
		return "", errors.New("query window cannot exceed 7 days")
	}
	fragments := []string{
		`resource.type = "cloud_run_revision"`,
		fmt.Sprintf(`timestamp >= %s`, quote(in.Start.UTC().Format(time.RFC3339Nano))),
		fmt.Sprintf(`timestamp < %s`, quote(in.End.UTC().Format(time.RFC3339Nano))),
	}
	if len(in.Sources) > 0 {
		choices := make([]string, 0, len(in.Sources))
		for _, source := range in.Sources {
			if !cloudRunServiceNamePattern.MatchString(source) {
				return "", errors.New("source must be a valid Cloud Run service name containing 1 to 49 lowercase letters, digits, or hyphens")
			}
			choices = append(choices, `resource.labels.service_name = `+quote(source))
		}
		if len(choices) > 0 {
			fragments = append(fragments, "("+strings.Join(choices, " OR ")+")")
		}
	}
	if len(in.Severities) > 0 {
		choices := make([]string, 0, len(in.Severities))
		for _, value := range in.Severities {
			severity, ok := canonicalSeverity(value)
			if !ok {
				return "", errors.New("invalid severity")
			}
			choices = append(choices, compileSeverity(severity))
		}
		fragments = append(fragments, "("+strings.Join(choices, " OR ")+")")
	}
	if strings.TrimSpace(in.Text) != "" {
		tokens, err := lex(in.Text)
		if err != nil {
			return "", err
		}
		if len(tokens) == 0 {
			return "", errors.New("query must contain a non-empty term")
		}
		compiled := make([]string, 0, len(tokens))
		for _, token := range tokens {
			fragment, err := token.compile()
			if err != nil {
				return "", err
			}
			compiled = append(compiled, fragment)
		}
		fragments = append(fragments, "("+strings.Join(compiled, " AND ")+")")
	}
	if native := strings.TrimSpace(in.NativeFilter); native != "" {
		if err := validateNativeFilter(native); err != nil {
			return "", err
		}
		fragments = append(fragments, native)
	}
	if len(in.Predicates) > MaxPredicates {
		return "", fmt.Errorf("structured query cannot contain more than %d predicates", MaxPredicates)
	}
	for i, predicate := range in.Predicates {
		fragment, err := compilePredicate(predicate)
		if err != nil {
			return "", fmt.Errorf("predicate %d: %w", i+1, err)
		}
		fragments = append(fragments, fragment)
	}
	for i := range fragments {
		fragments[i] = "(" + fragments[i] + ")"
	}
	filter := strings.Join(fragments, " AND ")
	if len(filter) > MaxFilterBytes {
		return "", fmt.Errorf("compiled filter exceeds the %d-byte limit; shorten the query or select fewer filters", MaxFilterBytes)
	}
	return filter, nil
}

var pathSegmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var cloudRunServiceNamePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,47}[a-z0-9])?$`)

func compilePredicate(predicate FieldPredicate) (string, error) {
	path := strings.TrimSpace(predicate.Path)
	path = strings.TrimPrefix(path, "jsonPayload.")
	if path == "" || path == "jsonPayload" || len(path) > 256 {
		return "", errors.New("path must contain 1 to 256 characters")
	}
	parts := strings.Split(path, ".")
	if len(parts) > 20 {
		return "", errors.New("path cannot contain more than 20 segments")
	}
	for _, part := range parts {
		if !pathSegmentPattern.MatchString(part) {
			return "", errors.New("path contains an invalid segment")
		}
	}
	field := "jsonPayload." + path
	switch predicate.Operator {
	case "equals":
		value, err := scalar(predicate.Value)
		if err != nil {
			return "", err
		}
		return field + " = " + value, nil
	case "contains":
		value, ok := predicate.Value.(string)
		if !ok || value == "" || len(value) > 2048 {
			return "", errors.New("contains value must be a string containing 1 to 2048 characters")
		}
		return field + ":" + quote(value), nil
	case "exists":
		value, ok := predicate.Value.(bool)
		if !ok {
			return "", errors.New("exists value must be a boolean")
		}
		if !value {
			return "NOT (" + field + ":*)", nil
		}
		return field + ":*", nil
	case "gt", "lt":
		value, ok := predicate.Value.(float64)
		if !ok || math.IsInf(value, 0) || math.IsNaN(value) {
			return "", fmt.Errorf("%s value must be a finite number", predicate.Operator)
		}
		operator := ">"
		if predicate.Operator == "lt" {
			operator = "<"
		}
		return field + " " + operator + " " + strconv.FormatFloat(value, 'g', -1, 64), nil
	default:
		return "", errors.New("operator must be one of equals, contains, exists, gt, or lt")
	}
}

func scalar(value any) (string, error) {
	switch value := value.(type) {
	case string:
		if len(value) > 2048 {
			return "", errors.New("string value cannot exceed 2048 characters")
		}
		return quote(value), nil
	case bool:
		return strconv.FormatBool(value), nil
	case float64:
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return "", errors.New("number value must be finite")
		}
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	default:
		return "", errors.New("equals value must be a string, number, or boolean")
	}
}

func CompileRequestContext(event time.Time, requestID, traceID string) (string, error) {
	return compileRequestContext(event, requestID, traceID, event.Add(-15*time.Minute), event.Add(15*time.Minute), "<=")
}

func CompileRequestContextBefore(event time.Time, requestID, traceID string) (string, error) {
	return compileRequestContext(event, requestID, traceID, event.Add(-15*time.Minute), event, "<")
}

func CompileRequestContextAfter(event time.Time, requestID, traceID string) (string, error) {
	return compileRequestContext(event, requestID, traceID, event, event.Add(15*time.Minute), "<=")
}

func compileRequestContext(event time.Time, requestID, traceID string, start, end time.Time, endOperator string) (string, error) {
	if event.IsZero() {
		return "", errors.New("eventTimestamp is required")
	}
	if strings.TrimSpace(requestID) == "" && strings.TrimSpace(traceID) == "" {
		return "", errors.New("requestId or traceId is required")
	}
	if requestID != strings.TrimSpace(requestID) || traceID != strings.TrimSpace(traceID) {
		return "", errors.New("context identifiers cannot have surrounding whitespace")
	}
	if len(requestID) > 2048 || len(traceID) > 2048 {
		return "", errors.New("context identifiers cannot exceed 2048 characters")
	}
	identifiers := make([]string, 0, 7)
	if requestID != "" {
		for _, field := range []string{"jsonPayload.requestId", "jsonPayload.request_id", "jsonPayload.request.id", "jsonPayload.httpRequest.requestId"} {
			identifiers = append(identifiers, field+" = "+quote(requestID))
		}
	}
	if traceID != "" {
		for _, field := range []string{"trace", "jsonPayload.trace", `jsonPayload."logging.googleapis.com/trace"`} {
			identifiers = append(identifiers, field+" = "+quote(traceID))
		}
	}
	fragments := []string{
		`resource.type = "cloud_run_revision"`,
		"timestamp >= " + quote(start.UTC().Format(time.RFC3339Nano)),
		"timestamp " + endOperator + " " + quote(end.UTC().Format(time.RFC3339Nano)),
		"(" + strings.Join(identifiers, " OR ") + ")",
	}
	for i := range fragments {
		fragments[i] = "(" + fragments[i] + ")"
	}
	filter := strings.Join(fragments, " AND ")
	if len(filter) > MaxFilterBytes {
		return "", fmt.Errorf("compiled filter exceeds the %d-byte limit; shorten the query or select fewer filters", MaxFilterBytes)
	}
	return filter, nil
}

func validateNativeFilter(filter string) error {
	depth := 0
	inQuote, escaped := false, false
	for _, r := range filter {
		if escaped {
			escaped = false
			continue
		}
		if inQuote && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return errors.New("native filter has unbalanced parentheses")
			}
		}
	}
	if inQuote || escaped {
		return errors.New("native filter has an unterminated quoted string")
	}
	if depth != 0 {
		return errors.New("native filter has unbalanced parentheses")
	}
	return nil
}

var severityOrder = []string{
	"DEFAULT", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY",
}

type token struct {
	value  string
	quoted bool
}

func lex(input string) ([]token, error) {
	var out []token
	var b strings.Builder
	inQuote, escaped, hasQuote, hasUnquoted := false, false, false, false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, token{value: b.String(), quoted: hasQuote && !hasUnquoted})
			b.Reset()
			hasQuote, hasUnquoted = false, false
		}
	}
	for _, r := range input {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if inQuote && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			hasQuote = true
			continue
		}
		if !inQuote {
			switch r {
			case ' ', '\t', '\n', '\r':
				flush()
				continue
			case '(', ')':
				return nil, errors.New("parentheses are not supported; terms are combined with implicit AND")
			}
		}
		if !inQuote {
			hasUnquoted = true
		}
		b.WriteRune(r)
	}
	if inQuote || escaped {
		return nil, errors.New("unterminated quoted string")
	}
	flush()
	for _, token := range out {
		if !token.quoted && (strings.EqualFold(token.value, "AND") || strings.EqualFold(token.value, "OR")) {
			return nil, errors.New("explicit AND and OR are not supported; separate terms with spaces")
		}
	}
	return out, nil
}

func (t token) compile() (string, error) {
	value := t.value
	if t.quoted {
		q := quote(value)
		return compileTextSearch(q), nil
	}
	negated := strings.HasPrefix(value, "-")
	if negated {
		value = strings.TrimPrefix(value, "-")
		if value == "" {
			return "", errors.New("negation requires a term")
		}
	}
	field, val, hasField := strings.Cut(value, ":")
	if negated && !hasField {
		return "", errors.New("negation is only supported for field:value terms")
	}
	if hasField && (field == "" || val == "") {
		return "", errors.New("field searches require a field and value")
	}
	var result string
	if !hasField {
		q := quote(value)
		result = compileTextSearch(q)
	} else {
		compiled, err := compileField(field, val)
		if err != nil {
			return "", err
		}
		result = compiled
	}
	if negated {
		result = "NOT (" + result + ")"
	}
	return result, nil
}

func compileField(field, value string) (string, error) {
	q := quote(value)
	switch strings.ToLower(field) {
	case "message", "msg":
		return compileTextSearch(q), nil
	case "service", "source":
		return `(resource.labels.service_name = ` + q + ` OR jsonPayload.service = ` + q + ` OR jsonPayload.service_name = ` + q + ` OR jsonPayload.service.name = ` + q + `)`, nil
	case "severity", "level":
		severity, ok := canonicalSeverity(value)
		if !ok {
			return "", errors.New("invalid severity")
		}
		return compileSeverity(severity), nil
	case "trace":
		return `(trace = ` + q + ` OR jsonPayload.trace = ` + q + ` OR jsonPayload."logging.googleapis.com/trace" = ` + q + `)`, nil
	case "status", "http.status_code":
		return `httpRequest.status = ` + q, nil
	}
	field = strings.TrimPrefix(field, "@")
	field = strings.TrimPrefix(field, "json.")
	for part := range strings.SplitSeq(field, ".") {
		if !pathSegmentPattern.MatchString(part) {
			return "", fmt.Errorf("invalid field %q", field)
		}
	}
	return "jsonPayload." + field + ` = ` + q, nil
}

func compileTextSearch(quotedValue string) string {
	return `(jsonPayload.message:` + quotedValue + ` OR jsonPayload.msg:` + quotedValue + ` OR jsonPayload.error.message:` + quotedValue + ` OR jsonPayload.exception.message:` + quotedValue + ` OR textPayload:` + quotedValue + `)`
}

func canonicalSeverity(value string) (string, bool) {
	severity := strings.ToUpper(strings.TrimSpace(value))
	if severity == "WARN" {
		severity = "WARNING"
	}
	if slices.Contains(severityOrder, severity) {
		return severity, true
	}
	return "", false
}

func compileSeverity(severity string) string {
	if severity == "DEFAULT" {
		nonDefaultLevels := make([]string, 0)
		for _, candidate := range severityOrder[1:] {
			nonDefaultLevels = append(nonDefaultLevels, jsonLevelMatcher(candidate))
		}
		return `(severity = "DEFAULT" AND NOT (` + strings.Join(nonDefaultLevels, " OR ") + `))`
	}
	choices := []string{`severity = ` + quote(severity)}
	choices = append(choices, `(severity = "DEFAULT" AND (`+jsonLevelMatcher(severity)+`))`)
	return "(" + strings.Join(choices, " OR ") + ")"
}

func jsonLevelMatcher(severity string) string {
	values := []string{severity}
	switch severity {
	case "DEBUG":
		values = append(values, "TRACE")
	case "WARNING":
		values = append(values, "WARN")
	case "CRITICAL":
		values = append(values, "FATAL", "PANIC")
	}
	pattern := values[0]
	if len(values) > 1 {
		pattern = "(?:" + strings.Join(values, "|") + ")"
	}
	// Cloud Logging's =~ operator uses RE2, whose inline (?i) flag provides exact case-insensitive matching.
	return `jsonPayload.level =~ ` + quote(`(?i)^`+pattern+`$`)
}

func quote(value string) string { return strconv.Quote(value) }
