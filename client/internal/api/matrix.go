package api

import "fmt"

const (
	ActionSubmitTask = "CVSync2AsyncSubmitTask"
	ActionGetResult  = "CVSync2AsyncGetResult"

	ReqKeyJimengT2IV40 = "jimeng_t2i_v40"
)

const (
	StatusInQueue   = "in_queue"
	StatusGenerating = "generating"
	StatusDone      = "done"
	StatusNotFound  = "not_found"
	StatusExpired   = "expired"
	StatusFailed    = "failed"
)

type FieldSpec struct {
	Name     string
	Type     string
	Required bool
	Note     string
}

type ApiActionSpec struct {
	CommandName     string
	Action          string
	ReqKey          string
	RequiredParams  []FieldSpec
	OptionalParams  []FieldSpec
	ResponseFields  []FieldSpec
	AllowedStatuses []string
}

type ApiMatrix struct {
	ActionOrder []string
	Actions     map[string]ApiActionSpec
}

func FrozenApiMatrix() ApiMatrix {
	statuses := []string{
		StatusInQueue,
		StatusGenerating,
		StatusDone,
		StatusNotFound,
		StatusExpired,
		StatusFailed,
	}

	return ApiMatrix{
		ActionOrder: []string{ActionSubmitTask, ActionGetResult},
		Actions: map[string]ApiActionSpec{
			ActionSubmitTask: {
				Action:          ActionSubmitTask,
				ReqKey:          ReqKeyJimengT2IV40,
				AllowedStatuses: statuses,
			},
			ActionGetResult: {
				Action:          ActionGetResult,
				ReqKey:          ReqKeyJimengT2IV40,
				AllowedStatuses: statuses,
			},
		},
	}
}

func (m ApiMatrix) ValidateAction(action string) error {
	if action == "" {
		return fmt.Errorf("action must not be empty")
	}

	for _, a := range m.ActionOrder {
		if a == action {
			return nil
		}
	}

	return fmt.Errorf("unsupported action: %s", action)
}
