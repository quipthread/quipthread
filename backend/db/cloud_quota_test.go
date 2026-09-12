package db

import (
	"fmt"
	"testing"

	"github.com/quipthread/quipthread/models"
)

func TestCloudQuotaBlocksPaidPlansAndDoesNotRefundDeletion(t *testing.T) {
	for _, tc := range []struct {
		plan  string
		limit int
	}{{"hobby", 1000}, {"starter", 10000}, {"pro", 50000}, {"business", 250000}} {
		t.Run(tc.plan, func(t *testing.T) {
			s, err := NewSQLiteStoreForTest(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			if err := s.UpsertSubscription(&models.Subscription{Plan: tc.plan, Status: "active"}); err != nil {
				t.Fatal(err)
			}
			if err := EnableCloudQuotas(t.Context(), s); err != nil {
				t.Fatal(err)
			}
			if err := s.CreateSite(&models.Site{ID: "site", OwnerID: "owner", Domain: "example.com"}); err != nil {
				t.Fatal(err)
			}
			if err := s.UpsertUser(&models.User{ID: "user", DisplayName: "User"}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.ExecContext(t.Context(), `INSERT INTO cloud_comment_usage(month, used) VALUES(strftime('%Y-%m','now'), ?) ON CONFLICT(month) DO UPDATE SET used=excluded.used`, tc.limit-1); err != nil {
				t.Fatal(err)
			}
			first := &models.Comment{ID: "first", SiteID: "site", UserID: "user", Content: "first", Status: "approved"}
			if err := s.CreateComment(first); err != nil {
				t.Fatal(err)
			}
			if err := s.DeleteComment(first.ID); err != nil {
				t.Fatal(err)
			}
			err = s.CreateComment(&models.Comment{ID: "next", SiteID: "site", UserID: "user", Content: "next", Status: "approved"})
			if QuotaErrorCode(err) != "monthly_limit_exceeded" {
				t.Fatalf("quota error = %v", err)
			}
			used, err := s.CountCommentsThisMonth()
			if err != nil || used != tc.limit {
				t.Fatalf("usage = %d, %v", used, err)
			}
		})
	}
}

func TestCloudSiteQuotaRejectsBeyondAllowance(t *testing.T) {
	s, err := NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := EnableCloudQuotas(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		err := s.CreateSite(&models.Site{ID: fmt.Sprint(i), OwnerID: "owner", Domain: "example.com"})
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i == 1 && QuotaErrorCode(err) != "site_limit_exceeded" {
			t.Fatalf("quota error = %v", err)
		}
	}
}
