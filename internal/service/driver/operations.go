package driver

import "strings"

const (
	OperationRender   = "render"
	OperationValidate = "validate"
	OperationApply    = "apply"
	OperationRestart  = "restart"
	OperationStart    = "start"
	OperationStop     = "stop"
	OperationEnable   = "enable"
	OperationDisable  = "disable"
)

type OperationSpec struct {
	Code                  string `json:"code"`
	JobType               string `json:"job_type,omitempty"`
	DisplayName           string `json:"display_name"`
	Category              string `json:"category"`
	ExecutionTarget       string `json:"execution_target"`
	RequiresSpec          bool   `json:"requires_spec"`
	RequiresRenderedFiles bool   `json:"requires_rendered_files"`
	RequiresSystemdUnit   bool   `json:"requires_systemd_unit"`
	MutatesConfig         bool   `json:"mutates_config"`
	MutatesRuntime        bool   `json:"mutates_runtime"`
	ConvergesDesiredState bool   `json:"converges_desired_state"`
	Destructive           bool   `json:"destructive"`
	AgentExecutable       bool   `json:"agent_executable"`
	QueuedStatus          string `json:"queued_status,omitempty"`
	SetsEnabled           bool   `json:"sets_enabled"`
	QueuedEnabled         bool   `json:"queued_enabled"`
}

var baseOperations = []OperationSpec{
	{
		Code:            OperationRender,
		DisplayName:     "Render config",
		Category:        "config",
		ExecutionTarget: "control-plane",
		RequiresSpec:    true,
		MutatesConfig:   false,
	},
	{
		Code:                  OperationValidate,
		DisplayName:           "Validate rendered config",
		Category:              "config",
		ExecutionTarget:       "agent",
		RequiresSpec:          true,
		RequiresRenderedFiles: true,
		AgentExecutable:       true,
	},
	{
		Code:                  OperationApply,
		JobType:               "instance.apply",
		DisplayName:           "Apply desired state",
		Category:              "convergence",
		ExecutionTarget:       "agent",
		RequiresSpec:          true,
		RequiresRenderedFiles: true,
		RequiresSystemdUnit:   true,
		MutatesConfig:         true,
		MutatesRuntime:        true,
		ConvergesDesiredState: true,
		AgentExecutable:       true,
		QueuedStatus:          "provisioning",
	},
	{
		Code:                OperationRestart,
		JobType:             "instance.restart",
		DisplayName:         "Restart runtime",
		Category:            "lifecycle",
		ExecutionTarget:     "agent",
		RequiresSystemdUnit: true,
		MutatesRuntime:      true,
		AgentExecutable:     true,
	},
	{
		Code:                OperationStart,
		JobType:             "instance.start",
		DisplayName:         "Start runtime",
		Category:            "lifecycle",
		ExecutionTarget:     "agent",
		RequiresSystemdUnit: true,
		MutatesRuntime:      true,
		AgentExecutable:     true,
		QueuedStatus:        "active",
		SetsEnabled:         true,
		QueuedEnabled:       true,
	},
	{
		Code:                OperationStop,
		JobType:             "instance.stop",
		DisplayName:         "Stop runtime",
		Category:            "lifecycle",
		ExecutionTarget:     "agent",
		RequiresSystemdUnit: true,
		MutatesRuntime:      true,
		Destructive:         true,
		AgentExecutable:     true,
		QueuedStatus:        "disabled",
		SetsEnabled:         true,
		QueuedEnabled:       false,
	},
	{
		Code:                  OperationEnable,
		JobType:               "instance.enable",
		DisplayName:           "Enable and start runtime",
		Category:              "lifecycle",
		ExecutionTarget:       "agent",
		RequiresSystemdUnit:   true,
		MutatesRuntime:        true,
		ConvergesDesiredState: true,
		AgentExecutable:       true,
		QueuedStatus:          "active",
		SetsEnabled:           true,
		QueuedEnabled:         true,
	},
	{
		Code:                  OperationDisable,
		JobType:               "instance.disable",
		DisplayName:           "Disable and stop runtime",
		Category:              "lifecycle",
		ExecutionTarget:       "agent",
		RequiresSystemdUnit:   true,
		MutatesRuntime:        true,
		ConvergesDesiredState: true,
		Destructive:           true,
		AgentExecutable:       true,
		QueuedStatus:          "disabled",
		SetsEnabled:           true,
		QueuedEnabled:         false,
	},
}

func NormalizeOperation(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "reload":
		return OperationRestart
	case "up":
		return OperationStart
	case "down":
		return OperationStop
	case "apply", "restart", "start", "stop", "enable", "disable", "render", "validate":
		return value
	default:
		return value
	}
}

func OperationsFor(code string) []OperationSpec {
	contract, ok := contracts[NormalizeCode(code)]
	if !ok {
		return nil
	}
	out := make([]OperationSpec, 0, len(baseOperations))
	for _, op := range baseOperations {
		if operationSupportedByContract(contract, op) {
			out = append(out, op)
		}
	}
	return out
}

func OperationFor(code, operation string) (OperationSpec, bool) {
	operation = NormalizeOperation(operation)
	for _, op := range OperationsFor(code) {
		if op.Code == operation {
			return op, true
		}
	}
	return OperationSpec{}, false
}

func SupportsOperation(code, operation string) bool {
	_, ok := OperationFor(code, operation)
	return ok
}

func OperationFromJobType(jobType string) (string, bool) {
	jobType = strings.TrimSpace(jobType)
	for _, op := range baseOperations {
		if op.JobType == jobType {
			return op.Code, true
		}
	}
	return "", false
}

func IsInstanceOperationJobType(jobType string) bool {
	_, ok := OperationFromJobType(jobType)
	return ok
}

func operationSupportedByContract(contract Contract, op OperationSpec) bool {
	if op.Code == OperationRender {
		return strings.TrimSpace(contract.DefaultConfigPath) != ""
	}
	if op.Code == OperationValidate {
		return strings.TrimSpace(contract.DefaultConfigPath) != ""
	}
	if op.JobType == "" {
		return true
	}
	return strings.TrimSpace(contract.DefaultUnitPattern) != ""
}
