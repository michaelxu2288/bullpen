package crew

import "fmt"

type PromptTemplate string

const (
	TemplatePlan   PromptTemplate = "plan"
	TemplateCode   PromptTemplate = "code"
	TemplateReview PromptTemplate = "review"
)

func RenderTemplate(t PromptTemplate, taskID, input string) string {
	switch t {
	case TemplatePlan:
		return fmt.Sprintf("[plan][%s]\nBreak this down into actionable steps:\n%s", taskID, input)
	case TemplateCode:
		return fmt.Sprintf("[code][%s]\nImplement and explain tradeoffs:\n%s", taskID, input)
	case TemplateReview:
		return fmt.Sprintf("[review][%s]\nFind risks/regressions first:\n%s", taskID, input)
	default:
		return input
	}
}
