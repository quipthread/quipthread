package config

import "testing"

func TestLoadTursoGroup(t *testing.T) {
	for _, tc := range []struct{ value, want string }{{"", "default"}, {"quipthread", "quipthread"}} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("TURSO_GROUP", tc.value)
			if got := Load().TursoGroup; got != tc.want {
				t.Fatalf("group = %q, want %q", got, tc.want)
			}
		})
	}
}
