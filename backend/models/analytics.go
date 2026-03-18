package models

type VolumePoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type PageStat struct {
	PageID    string `json:"page_id"`
	PageTitle string `json:"page_title"`
	Count     int    `json:"count"`
}

type CommenterStat struct {
	DisplayName string `json:"display_name"`
	Count       int    `json:"count"`
}

// Pro+ fields

type StatusStat struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type PeakHourStat struct {
	Hour  int `json:"hour"` // 0–23
	Count int `json:"count"`
}

type PeakDayStat struct {
	Day   int `json:"day"` // 0=Sunday … 6=Saturday
	Count int `json:"count"`
}

// AnalyticsResult holds all tiers of analytics data. Fields are nil/omitted
// when the caller's plan does not include them.
type AnalyticsResult struct {
	// Starter+
	Volume     []VolumePoint   `json:"volume"`
	Pages      []PageStat      `json:"pages"`
	Commenters []CommenterStat `json:"commenters"`

	// Pro+  (nil when not computed)
	StatusBreakdown []StatusStat   `json:"status_breakdown,omitempty"`
	PeakHours       []PeakHourStat `json:"peak_hours,omitempty"`
	PeakDays        []PeakDayStat  `json:"peak_days,omitempty"`

	// Business+  (nil when not computed)
	ReturnRate *float64 `json:"return_rate,omitempty"` // 0–100
}
