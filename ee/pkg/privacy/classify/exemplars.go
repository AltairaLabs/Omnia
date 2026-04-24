/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import "github.com/altairalabs/omnia/ee/pkg/privacy"

// categoryExemplars is the per-category list of representative sentences
// used to compute centroids for the embedding classifier.
//
// Tuning notes:
//   - Aim for ~10 exemplars per category covering the semantic range.
//   - Avoid exemplars that overlap other categories (e.g. "I prefer to
//     live in Edinburgh" is bad — it straddles preferences and location).
//   - analytics:aggregate is intentionally absent: it's an operator
//     consent axis, not a content-derivable category.
var categoryExemplars = map[privacy.ConsentCategory][]string{
	privacy.ConsentMemoryHealth: {
		"I'm allergic to peanuts",
		"I take 50mg of sertraline daily",
		"diagnosed with type 2 diabetes",
		"I have anxiety and depression",
		"I get migraines often",
		"I'm gluten intolerant",
		"vegan for medical reasons",
		"my blood pressure is high",
		"I have ADHD",
		"recovering from surgery",
	},
	privacy.ConsentMemoryIdentity: {
		"my name is Charlie",
		"you can reach me at charlie@example.com",
		"I'm a senior engineer at Acme",
		"my pronouns are they/them",
		"I'm 35 years old",
		"my employee ID is 12345",
		"I go by Charlie professionally",
		"I'm the CEO of Initech",
		"my date of birth is 12 May 1988",
		"my full legal name is Charles Holland",
	},
	privacy.ConsentMemoryLocation: {
		"I live in Edinburgh",
		"based in San Francisco",
		"my office is in London",
		"currently working from Berlin",
		"my postcode is EH1 1AA",
		"I'm in the UTC+1 timezone",
		"my address is 42 Main Street",
		"working remotely from Lisbon today",
		"I commute from Brighton to London",
		"my home is in the Scottish Highlands",
	},
	privacy.ConsentMemoryPreferences: {
		"I prefer dark mode",
		"my favourite colour is blue",
		"I like coffee in the morning",
		"I always use vim",
		"I hate noisy environments",
		"my preferred meeting time is afternoons",
		"I dislike spicy food",
		"I love jazz music",
		"my preferred IDE is VSCode",
		"I like Python over Java",
	},
	privacy.ConsentMemoryContext: {
		"working on the Q3 roadmap",
		"the migration to GCP is blocked",
		"this is for the user-auth project",
		"currently in sprint planning",
		"the team is shipping next Tuesday",
		"we're refactoring the payments service",
		"deadline is end of quarter",
		"reviewing the architecture proposal",
		"my current sprint focuses on observability",
		"the staging environment is broken",
	},
	privacy.ConsentMemoryHistory: {
		"we discussed this last week",
		"earlier you mentioned the API redesign",
		"in our previous conversation about deployment",
		"last time we talked about budget",
		"as I said before, the deadline is fixed",
		"recall the bug we fixed yesterday",
		"following up on our chat from Monday",
		"continuing where we left off",
		"per our earlier discussion",
		"we agreed last sprint to defer this",
	},
}
