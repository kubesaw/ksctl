package assets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldBeSkippedForMember1(t *testing.T) {
	// given
	member1Member2 := []string{"member1", "member2"}
	member2Member3 := []string{"member2", "member3"}
	testCases := map[string]struct {
		s               Selector
		shouldBeSkipped bool
	}{
		"no selector":                   {s: Selector{}, shouldBeSkipped: false},
		"different selected members":    {s: Selector{MemberClusters: member2Member3}, shouldBeSkipped: true},
		"in selected members":           {s: Selector{MemberClusters: member1Member2}, shouldBeSkipped: false},
		"listed in skipped members":     {s: Selector{SkipMembers: member1Member2}, shouldBeSkipped: true},
		"not listed in skipped members": {s: Selector{SkipMembers: member2Member3}, shouldBeSkipped: false},
		"in selected members, but listed in skipped": {
			s: Selector{MemberClusters: member1Member2, SkipMembers: member1Member2}, shouldBeSkipped: true},
		"in selected members, not listed in skipped": {
			s: Selector{MemberClusters: member1Member2, SkipMembers: member2Member3}, shouldBeSkipped: false},
		"different selected members, not listed in skipped": {
			s: Selector{MemberClusters: member2Member3, SkipMembers: member2Member3}, shouldBeSkipped: true},
		"different selected members, and listed in skipped": {
			s: Selector{MemberClusters: member2Member3, SkipMembers: member1Member2}, shouldBeSkipped: true},
	}

	for testName, data := range testCases {
		t.Run(testName, func(t *testing.T) {
			// when
			shouldBeSkipped := data.s.ShouldBeSkippedForMember("member1")

			// then
			assert.Equal(t, data.shouldBeSkipped, shouldBeSkipped)
		})
	}
}

func TestShouldBeSkippedForEmptyName(t *testing.T) {
	// given
	member1Member2 := []string{"member1", "member2"}
	testCases := map[string]struct {
		s               Selector
		shouldBeSkipped bool
	}{
		"no selector":           {s: Selector{}, shouldBeSkipped: false},
		"some selected members": {s: Selector{MemberClusters: member1Member2}, shouldBeSkipped: true},
		"some skipped members":  {s: Selector{SkipMembers: member1Member2}, shouldBeSkipped: false},
		"some selected members and some skipped members": {
			s: Selector{MemberClusters: member1Member2, SkipMembers: member1Member2}, shouldBeSkipped: true},
	}

	for testName, data := range testCases {
		t.Run(testName, func(t *testing.T) {
			// when
			shouldBeSkipped := data.s.ShouldBeSkippedForMember("")

			// then
			assert.Equal(t, data.shouldBeSkipped, shouldBeSkipped)
		})
	}
}
