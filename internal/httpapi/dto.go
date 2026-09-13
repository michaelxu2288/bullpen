package httpapi

type RunGoalRequest struct {
	Goal    string `json:"goal"`
	TraceID string `json:"trace_id"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
