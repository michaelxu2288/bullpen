package plan

import (
	"strings"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/runners"
)

type Router struct {
	PlannerSession  string
	CoderSession    string
	ReviewerSession string
}

func (r Router) RouteTask(task domain.Task) (session string, role runners.Role) {
	title := strings.ToLower(task.Title)
	desc := strings.ToLower(task.Description)

	if strings.Contains(title, "review") || strings.Contains(desc, "review") {
		return r.ReviewerSession, runners.RoleReviewer
	}
	if strings.Contains(title, "plan") || strings.Contains(desc, "decompose") {
		return r.PlannerSession, runners.RolePlanner
	}
	return r.CoderSession, runners.RoleCoder
}
