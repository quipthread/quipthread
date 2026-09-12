package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestImportMultipartLimitsAreOuterRequestLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		max  int64
	}{
		{name: "text", max: maxTextImportBodyBytes},
		{name: "sqlite", max: maxSQLiteImportBodyBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := bytes.NewReader(nil)
			req := httptest.NewRequest("POST", "/import", body)
			req.ContentLength = tc.max + 1
			rec := httptest.NewRecorder()
			err := parseImportMultipart(rec, req, tc.max)
			if !importBodyTooLarge(err) {
				t.Fatalf("parseImportMultipart error = %v, want body-too-large", err)
			}
		})
	}
}

func TestModRulesJSONLimitReadsThroughEndOfBody(t *testing.T) {
	body := append([]byte(`{"term":"ok"}`), bytes.Repeat([]byte(" "), int(maxNormalJSONBodyBytes))...)
	req := httptest.NewRequest("POST", "/modrules", bytes.NewReader(body))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	var dst struct {
		Term string `json:"term"`
	}
	err := decodeModRulesJSON(rec, req, &dst)
	if !modRulesBodyTooLarge(err) {
		t.Fatalf("decodeModRulesJSON error = %v, want body-too-large", err)
	}
}

func TestModRulesJSONLimitAllowsExactLimit(t *testing.T) {
	prefix, suffix := []byte(`{"term":"`), []byte(`"}`)
	body := append(append(append([]byte(nil), prefix...), bytes.Repeat([]byte("x"), int(maxNormalJSONBodyBytes)-len(prefix)-len(suffix))...), suffix...)
	req := httptest.NewRequest("POST", "/modrules", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	var dst struct {
		Term string `json:"term"`
	}
	if err := decodeModRulesJSON(rec, req, &dst); err != nil {
		t.Fatalf("decodeModRulesJSON exact-limit error = %v", err)
	}
}
