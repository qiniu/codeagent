package models

// Constants related to three-section output
const (
	// Section headers
	SectionSummary  = "## Change Summary"
	SectionChanges  = "## Specific Changes"
	SectionTestPlan = "## Test Plan"

	// Section identifiers (for parsing)
	SectionSummaryID  = "summary"
	SectionChangesID  = "changes"
	SectionTestPlanID = "testPlan"

	// Error identifiers
	ErrorPrefixError     = "error:"
	ErrorPrefixException = "exception:"
	ErrorPrefixTraceback = "traceback:"
	ErrorPrefixPanic     = "panic:"
)
