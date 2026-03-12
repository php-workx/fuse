package core_test

import (
	"regexp"
	"testing"

	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/policy"
)

func TestGoldenFixtures_HardcodedBlockedCoverage(t *testing.T) {
	fixtures := loadGoldenFixtures(t)
	covered := make([]int, len(policy.HardcodedBlocked))

	for _, fixture := range fixtures.Fixtures {
		if fixture.Expected != string(core.DecisionBlocked) {
			continue
		}
		for i, rule := range policy.HardcodedBlocked {
			if rule.Pattern.MatchString(fixture.Command) {
				covered[i]++
			}
		}
	}

	for i, rule := range policy.HardcodedBlocked {
		if covered[i] == 0 {
			t.Errorf("hardcoded rule %d has no BLOCKED golden fixture: %s (%s)", i, rule.Reason, rule.Pattern.String())
		}
	}
}

func TestGoldenFixtures_HighRiskFamilyCoverage(t *testing.T) {
	fixtures := loadGoldenFixtures(t)

	type familyExpectation struct {
		name          string
		pattern       *regexp.Regexp
		minTotal      int
		requiredByDec map[string]int
	}

	families := []familyExpectation{
		{
			name:     "terraform_tofu",
			pattern:  regexp.MustCompile(`\b(terraform|tofu)\b`),
			minTotal: 8,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     2,
				string(core.DecisionCaution):  1,
				string(core.DecisionApproval): 5,
			},
		},
		{
			name:     "pulumi",
			pattern:  regexp.MustCompile(`\bpulumi\b`),
			minTotal: 5,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     1,
				string(core.DecisionApproval): 4,
			},
		},
		{
			name:     "kubernetes_helm",
			pattern:  regexp.MustCompile(`\b(kubectl|helm)\b`),
			minTotal: 4,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     1,
				string(core.DecisionApproval): 3,
			},
		},
		{
			name:     "aws",
			pattern:  regexp.MustCompile(`\baws\b`),
			minTotal: 10,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     1,
				string(core.DecisionCaution):  2,
				string(core.DecisionApproval): 6,
			},
		},
		{
			name:     "gcloud",
			pattern:  regexp.MustCompile(`\bgcloud\b`),
			minTotal: 7,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     1,
				string(core.DecisionApproval): 6,
			},
		},
		{
			name:     "azure",
			pattern:  regexp.MustCompile(`\baz\b`),
			minTotal: 5,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     1,
				string(core.DecisionApproval): 4,
			},
		},
		{
			name:     "docker",
			pattern:  regexp.MustCompile(`\bdocker\b`),
			minTotal: 8,
			requiredByDec: map[string]int{
				string(core.DecisionSafe):     3,
				string(core.DecisionCaution):  2,
				string(core.DecisionApproval): 3,
			},
		},
	}

	for _, family := range families {
		family := family
		t.Run(family.name, func(t *testing.T) {
			total := 0
			byDecision := map[string]int{}

			for _, fixture := range fixtures.Fixtures {
				if !family.pattern.MatchString(fixture.Command) {
					continue
				}
				total++
				byDecision[fixture.Expected]++
			}

			if total < family.minTotal {
				t.Fatalf("expected at least %d fixtures for %s, got %d", family.minTotal, family.name, total)
			}

			for decision, minCount := range family.requiredByDec {
				if byDecision[decision] < minCount {
					t.Errorf("expected at least %d %s fixtures for %s, got %d", minCount, decision, family.name, byDecision[decision])
				}
			}
		})
	}
}
