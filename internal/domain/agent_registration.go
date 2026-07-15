package domain

import "errors"

var ErrAgentRegistrationInvalidCredential = errors.New("agent registration credential is invalid")
var ErrAgentRegistrationNodeNotFound = errors.New("agent registration node not found")
var ErrAgentRegistrationNodeRetired = errors.New("agent registration node is retired")
var ErrAgentRegistrationAuditFailed = errors.New("agent registration audit failed")
