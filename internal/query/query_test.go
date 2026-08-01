package query

import (
	"strings"
	"testing"
	"time"
)

func TestCompileStructuredPredicates(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	filter, err := Compile(CompileInput{
		Sources: []string{"api"}, Severities: []string{"error"}, Start: start, End: start.Add(time.Hour),
		Predicates: []FieldPredicate{
			{Path: "jsonPayload.request.id", Operator: "equals", Value: `req-"quoted"`},
			{Path: "message", Operator: "contains", Value: "timeout"},
			{Path: "retryable", Operator: "exists", Value: false},
			{Path: "duration_ms", Operator: "gt", Value: float64(100)},
			{Path: "sample_rate", Operator: "lt", Value: 0.5},
			{Path: "successful", Operator: "equals", Value: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`resource.type = "cloud_run_revision"`, `resource.labels.service_name = "api"`, `severity = "ERROR"`,
		`jsonPayload.request.id = "req-\"quoted\""`, `jsonPayload.message:"timeout"`,
		`NOT (jsonPayload.retryable:*)`, `jsonPayload.duration_ms > 100`, `jsonPayload.sample_rate < 0.5`,
		`jsonPayload.successful = true`,
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter missing %q:\n%s", want, filter)
		}
	}
}

func TestCompileRejectsMalformedStructuredPredicates(t *testing.T) {
	now := time.Now()
	tests := []FieldPredicate{
		{Path: "", Operator: "equals", Value: "x"},
		{Path: "request..id", Operator: "equals", Value: "x"},
		{Path: "request/id", Operator: "equals", Value: "x"},
		{Path: "request.id", Operator: "unknown", Value: "x"},
		{Path: "request.id", Operator: "contains", Value: true},
		{Path: "request.id", Operator: "gt", Value: "100"},
		{Path: "request.id", Operator: "exists", Value: "true"},
		{Path: "request.id", Operator: "equals", Value: nil},
		{Path: "request.id", Operator: "equals", Value: []string{"x"}},
	}
	for _, predicate := range tests {
		if _, err := Compile(CompileInput{Predicates: []FieldPredicate{predicate}, Start: now, End: now.Add(time.Hour)}); err == nil {
			t.Errorf("accepted malformed predicate %#v", predicate)
		}
	}
	tooMany := make([]FieldPredicate, MaxPredicates+1)
	for i := range tooMany {
		tooMany[i] = FieldPredicate{Path: "value", Operator: "equals", Value: "x"}
	}
	if _, err := Compile(CompileInput{Predicates: tooMany, Start: now, End: now.Add(time.Hour)}); err == nil {
		t.Fatalf("accepted more than %d predicates", MaxPredicates)
	}
}

func TestStructuredPathAcceptsJsonPayloadPrefix(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{
		Predicates: []FieldPredicate{{Path: "jsonPayload.level", Operator: "equals", Value: "INFO"}},
		Start:      now,
		End:        now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filter, `jsonPayload.level = "INFO"`) {
		t.Fatalf("prefixed structured path compiled incorrectly: %s", filter)
	}
}

func TestCompileRequestContextUsesOnlyKnownExactLocations(t *testing.T) {
	event := time.Date(2026, 3, 4, 12, 30, 0, 0, time.UTC)
	filter, err := CompileRequestContext(event, `req-"quoted"`, "projects/example-project/traces/trace-123")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`resource.type = "cloud_run_revision"`,
		`timestamp >= "2026-03-04T12:15:00Z"`, `timestamp <= "2026-03-04T12:45:00Z"`,
		`jsonPayload.requestId = "req-\"quoted\""`, `jsonPayload.request_id = "req-\"quoted\""`,
		`jsonPayload.request.id = "req-\"quoted\""`, `jsonPayload.httpRequest.requestId = "req-\"quoted\""`,
		`trace = "projects/example-project/traces/trace-123"`,
		`jsonPayload.trace = "projects/example-project/traces/trace-123"`,
		`jsonPayload."logging.googleapis.com/trace" = "projects/example-project/traces/trace-123"`,
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("context filter missing %q:\n%s", want, filter)
		}
	}
	if strings.Contains(filter, "service_name") || strings.Contains(filter, "severity") {
		t.Fatalf("context filter inherited active query scope: %s", filter)
	}
}

func TestCompileRequestContextRequiresKnownIdentifier(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		event              time.Time
		requestID, traceID string
	}{
		{event: now},
		{requestID: "request-1"},
		{event: now, requestID: " "},
	} {
		if _, err := CompileRequestContext(tc.event, tc.requestID, tc.traceID); err == nil {
			t.Fatalf("accepted invalid context input %#v", tc)
		}
	}
}

func TestCompileSyntaxAndEscaping(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	filter, err := Compile(CompileInput{
		Text:         `timeout "connection reset" service:checkout -@http.method:POST severity:error`,
		Sources:      []string{"api"},
		Severities:   []string{"warning"},
		Start:        start,
		End:          start.Add(time.Hour),
		NativeFilter: `labels.environment="test" OR labels.environment="dev"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`(resource.type = "cloud_run_revision")`,
		`(timestamp >= "2026-01-02T03:04:05Z")`, `resource.labels.service_name = "api"`,
		`severity = "WARNING"`, `jsonPayload.level = "WARN"`, `jsonPayload.http.method = "POST"`,
		`severity = "ERROR"`, `jsonPayload.level = "ERROR"`,
		`(labels.environment="test" OR labels.environment="dev")`,
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter missing %q:\n%s", want, filter)
		}
	}
}

func TestCompileFormalErrors(t *testing.T) {
	now := time.Now()
	for _, text := range []string{`"unterminated`, `""`, `(foo`, `foo OR bar`, `foo AND bar`, `field:`, `-`, `-free`, `foo )`} {
		if _, err := Compile(CompileInput{Text: text, Start: now, End: now.Add(time.Hour)}); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	if _, err := Compile(CompileInput{Start: now, End: now.Add(MaxWindow + time.Second)}); err == nil {
		t.Fatal("accepted overlong window")
	}
}

func TestQuotedAndImplicitAND(t *testing.T) {
	tokens, err := lex(`service:"my api" first second`)
	if err != nil || len(tokens) != 3 || tokens[0].value != "service:my api" || tokens[0].quoted {
		t.Fatalf("tokens %#v, %v", tokens, err)
	}
	filter, err := Compile(CompileInput{Text: `"AND" "OR" "service:checkout" service:"my api" "service":worker`, Start: time.Now(), End: time.Now().Add(time.Hour)})
	if err != nil || !strings.Contains(filter, `jsonPayload.message:"AND"`) {
		t.Fatalf("quoted operators were not treated as text: %q, %v", filter, err)
	}
	if !strings.Contains(filter, `jsonPayload.message:"service:checkout"`) || strings.Contains(filter, `resource.labels.service_name = "checkout"`) {
		t.Fatalf("fully quoted field-like token was not treated as text: %s", filter)
	}
	for _, want := range []string{`resource.labels.service_name = "my api"`, `resource.labels.service_name = "worker"`} {
		if !strings.Contains(filter, want) {
			t.Fatalf("mixed quoted token was not treated as a field search; missing %q: %s", want, filter)
		}
	}
}

func TestTextQueriesUseOnlyScalarPayloadFields(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{Text: `starting "worker ready" message:booting`, Start: now, End: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"jsonPayload.message", "jsonPayload.msg", "textPayload"} {
		if !strings.Contains(filter, field) {
			t.Fatalf("text filter missing %s: %s", field, filter)
		}
	}
	if strings.Contains(filter, "protoPayload:") {
		t.Fatalf("text filter compares the protoPayload object as a scalar: %s", filter)
	}
}

func TestSeverityPayloadFallbackRequiresDefaultTopLevelSeverity(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{Severities: []string{"error"}, Start: now, End: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filter, `(severity = "DEFAULT" AND (jsonPayload.level = "ERROR"`) {
		t.Fatalf("payload fallback is not gated by DEFAULT severity: %s", filter)
	}
	if strings.Contains(filter, ` OR jsonPayload.level = "ERROR"`) {
		t.Fatalf("payload fallback can match conflicting top-level severity: %s", filter)
	}
}

func TestDefaultSeverityExcludesRecognizedPayloadLevels(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{Severities: []string{"DEFAULT"}, Start: now, End: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{`jsonPayload.level = "ERROR"`, `jsonPayload.level = "WARN"`, `jsonPayload.level = "INFO"`} {
		if !strings.Contains(filter, excluded) {
			t.Fatalf("DEFAULT filter does not exclude recognized payload level %q: %s", excluded, filter)
		}
	}
	if !strings.Contains(filter, `severity = "DEFAULT" AND NOT (`) {
		t.Fatalf("DEFAULT filter is not gated against recognized payload levels: %s", filter)
	}
}

func TestLeopardFieldsUseStructuredPathSegmentGrammar(t *testing.T) {
	now := time.Now()
	for _, text := range []string{"request-id:value", "request/id:value", "request..id:value", `request["id"]:value`} {
		if _, err := Compile(CompileInput{Text: text, Start: now, End: now.Add(time.Hour)}); err == nil {
			t.Errorf("accepted field with special path syntax %q", text)
		}
	}
	filter, err := Compile(CompileInput{Text: "request.id:value", Start: now, End: now.Add(time.Hour)})
	if err != nil || !strings.Contains(filter, `jsonPayload.request.id = "value"`) {
		t.Fatalf("valid segmented path failed: %q, %v", filter, err)
	}
}

func TestSourceAndSeverityChoicesStayGrouped(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{
		Sources: []string{"api", "worker"}, Severities: []string{"warning", "error"},
		Start: now, End: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`((resource.labels.service_name = "api" OR resource.labels.service_name = "worker"))`,
		`severity = "WARNING"`, `jsonPayload.level = "WARN"`, `severity = "ERROR"`,
	} {
		if !strings.Contains(filter, want) {
			t.Fatalf("filter missing grouped fragment %q: %s", want, filter)
		}
	}
}

func TestAliasesSearchProviderAndStructuredLocations(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{Text: `service:api severity:warn`, Start: now, End: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`resource.labels.service_name = "api"`, `jsonPayload.service = "api"`,
		`jsonPayload.service_name = "api"`, `jsonPayload.service.name = "api"`,
		`severity = "WARNING"`, `jsonPayload.level = "WARN"`, `jsonPayload.level = "WARNING"`,
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter missing %q: %s", want, filter)
		}
	}
}

func TestCloudRunScopeCannotBeEscapedByNativeFilter(t *testing.T) {
	now := time.Now()
	filter, err := Compile(CompileInput{NativeFilter: `severity="DEBUG" OR resource.type="gce_instance"`, Start: now, End: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	want := `(resource.type = "cloud_run_revision") AND (timestamp >= `
	if !strings.Contains(filter, want) || !strings.HasSuffix(filter, `(severity="DEBUG" OR resource.type="gce_instance")`) {
		t.Fatalf("native filter escaped fixed scope: %s", filter)
	}
	for _, native := range []string{
		`severity="DEBUG") OR resource.type="gce_instance" OR (resource.type="cloud_run_revision"`,
		`textPayload="unterminated`,
	} {
		if _, err := Compile(CompileInput{NativeFilter: native, Start: now, End: now.Add(time.Hour)}); err == nil {
			t.Fatalf("accepted native filter that could escape its enclosing group: %s", native)
		}
	}
}
