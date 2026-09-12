package cloud

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestTeamMemberJSONUsesDashboardFieldNames(t *testing.T) {
	t.Parallel()
	invitedAt := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)
	acceptedAt := invitedAt.Add(time.Hour)
	for _, tc := range []struct {
		name       string
		accepted   bool
		acceptedAt *time.Time
		wantTime   json.RawMessage
	}{
		{"pending", false, nil, json.RawMessage(`null`)},
		{"accepted", true, &acceptedAt, json.RawMessage(`"2026-09-09T01:00:00Z"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			member := TeamMember{
				ID: "member-1", AccountID: "account-1", Email: "reader@example.com",
				Role: "admin", InviteToken: "fixture-token", Accepted: tc.accepted,
				InvitedAt: invitedAt, AcceptedAt: tc.acceptedAt,
			}
			wantAccepted := json.RawMessage(`false`)
			if tc.accepted {
				wantAccepted = json.RawMessage(`true`)
			}
			want := map[string]json.RawMessage{
				"id": json.RawMessage(`"member-1"`), "account_id": json.RawMessage(`"account-1"`),
				"email": json.RawMessage(`"reader@example.com"`), "role": json.RawMessage(`"admin"`),
				"invite_token": json.RawMessage(`"fixture-token"`), "accepted": wantAccepted,
				"invited_at": json.RawMessage(`"2026-09-09T00:00:00Z"`), "accepted_at": tc.wantTime,
			}

			encoded, err := json.Marshal(member)

			if err != nil {
				t.Fatal(err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("dashboard JSON = %s; want snake_case fields with preserved values", encoded)
			}
		})
	}
}
