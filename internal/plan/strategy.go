package plan

import "github.com/michaelxu2288/bullpen/internal/domain"

type Strategy string

const (
	StrategyFastLane     Strategy = "fast_lane"
	StrategyReviewHeavy  Strategy = "review_heavy"
	StrategyParallelPair Strategy = "parallel_pair"
)

type StrategyPlanner struct{}

func (s StrategyPlanner) Select(tasks []domain.Task) Strategy {
	hasReview := false
	for _, t := range tasks {
		if t.RequiresApproval {
			hasReview = true
			break
		}
	}
	if hasReview {
		return StrategyReviewHeavy
	}
	if len(tasks) >= 4 {
		return StrategyParallelPair
	}
	return StrategyFastLane
}
