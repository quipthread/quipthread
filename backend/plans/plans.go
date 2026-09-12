package plans

import "github.com/quipthread/quipthread/models"

type Limits struct {
	Comments int
	Sites    int
	Rank     int
}

func Catalog() map[string]Limits {
	return map[string]Limits{
		"hobby":      {1000, 1, 0},
		"starter":    {10000, 5, 1},
		"pro":        {50000, 20, 2},
		"business":   {250000, -1, 3},
		"enterprise": {-1, -1, 4},
	}
}

func Effective(sub *models.Subscription) string {
	if sub == nil {
		return "hobby"
	}
	if sub.Status != "active" && sub.Status != "trialing" {
		return "hobby"
	}
	if _, ok := Catalog()[sub.Plan]; !ok {
		return "hobby"
	}
	return sub.Plan
}
