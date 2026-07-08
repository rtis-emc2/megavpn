package postgres

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeJSONFieldRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	err := decodeJSONField([]byte(`{"bad":`), &payload, "jobs.payload_json")
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !strings.Contains(err.Error(), "jobs.payload_json") {
		t.Fatalf("error = %q, want field name", err.Error())
	}
}

func TestDecodeJSONFieldAllowsEmptyJSON(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"kept": true}
	if err := decodeJSONField(nil, &payload, "empty"); err != nil {
		t.Fatalf("decodeJSONField returned error for nil JSON: %v", err)
	}
	if payload["kept"] != true {
		t.Fatalf("payload was modified: %#v", payload)
	}
}

func TestScanJobRejectsMalformedPayloadJSON(t *testing.T) {
	t.Parallel()

	_, err := scanJob(staticJobScanner{payload: []byte(`{"bad":`), result: []byte(`{}`)})
	if err == nil {
		t.Fatal("expected malformed job payload error")
	}
	if !strings.Contains(err.Error(), "jobs.payload_json") {
		t.Fatalf("error = %q, want jobs.payload_json", err.Error())
	}
}

func TestScanJobRejectsMalformedResultJSON(t *testing.T) {
	t.Parallel()

	_, err := scanJob(staticJobScanner{payload: []byte(`{}`), result: []byte(`{"bad":`)})
	if err == nil {
		t.Fatal("expected malformed job result error")
	}
	if !strings.Contains(err.Error(), "jobs.result_json") {
		t.Fatalf("error = %q, want jobs.result_json", err.Error())
	}
}

type staticJobScanner struct {
	payload []byte
	result  []byte
}

func (s staticJobScanner) Scan(dest ...any) error {
	now := time.Now().UTC()
	values := []any{
		"job-1",
		"node.test",
		"node",
		nil,
		nil,
		nil,
		"queued",
		10,
		s.payload,
		s.result,
		nil,
		nil,
		now,
		nil,
		nil,
	}
	for idx, value := range values {
		switch target := dest[idx].(type) {
		case *string:
			if value == nil {
				*target = ""
				continue
			}
			*target = value.(string)
		case **string:
			if value == nil {
				*target = nil
				continue
			}
			v := value.(string)
			*target = &v
		case *int:
			*target = value.(int)
		case *[]byte:
			*target = value.([]byte)
		case **time.Time:
			if value == nil {
				*target = nil
				continue
			}
			v := value.(time.Time)
			*target = &v
		case *time.Time:
			*target = value.(time.Time)
		default:
			panic("unsupported scan target")
		}
	}
	return nil
}
