package main

import (
	"testing"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/models"
)

func TestSameDatabaseDetectsEquivalentConfigs(t *testing.T) {
	a := config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, Name: "spindle_edge"}
	b := config.DatabaseConfig{Host: " 127.0.0.1 ", Port: 3306, Name: "SPINDLE_EDGE"}
	if !sameDatabase(a, b) {
		t.Fatal("expected equivalent host/port/name to be treated as same database")
	}
}

func TestSameDatabaseRejectsDifferentDatabase(t *testing.T) {
	base := config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, Name: "spindle_main"}
	cases := []config.DatabaseConfig{
		{Host: "127.0.0.2", Port: 3306, Name: "spindle_main"},
		{Host: "127.0.0.1", Port: 3307, Name: "spindle_main"},
		{Host: "127.0.0.1", Port: 3306, Name: "spindle_edge"},
	}
	for _, other := range cases {
		if sameDatabase(base, other) {
			t.Fatalf("expected different config to be treated as separate database: %+v", other)
		}
	}
}

func TestStandardMatchesProject(t *testing.T) {
	project := models.Project{ID: 7, ProjectCode: "AC-01", ProjectGroup: "AC"}
	projectID := uint(7)
	otherProjectID := uint(8)
	cases := []struct {
		name     string
		standard models.DetectionStandard
		want     bool
	}{
		{name: "global standard", standard: models.DetectionStandard{}, want: true},
		{name: "matching id", standard: models.DetectionStandard{ProjectID: &projectID}, want: true},
		{name: "matching code", standard: models.DetectionStandard{ProjectCode: "ac-01"}, want: true},
		{name: "matching group", standard: models.DetectionStandard{ProjectGroup: "ac"}, want: true},
		{name: "different id", standard: models.DetectionStandard{ProjectID: &otherProjectID}, want: false},
		{name: "different code", standard: models.DetectionStandard{ProjectCode: "AC-02"}, want: false},
		{name: "different group", standard: models.DetectionStandard{ProjectGroup: "OTHER"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := standardMatchesProject(tc.standard, project); got != tc.want {
				t.Fatalf("standardMatchesProject()=%v want %v", got, tc.want)
			}
		})
	}
}
