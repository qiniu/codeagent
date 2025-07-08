package models

// 三段式输出相关的常量
const (
	// 章节标题
	SectionSummary  = "## 改动摘要"
	SectionChanges  = "## 具体改动"
	SectionTestPlan = "## 测试计划"

	// 章节标识符（用于解析）
	SectionSummaryID  = "summary"
	SectionChangesID  = "changes"
	SectionTestPlanID = "testPlan"

	// 错误标识符
	ErrorPrefixError     = "error:"
	ErrorPrefixException = "exception:"
	ErrorPrefixTraceback = "traceback:"
	ErrorPrefixPanic     = "panic:"
)
