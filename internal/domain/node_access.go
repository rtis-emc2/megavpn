package domain

import "errors"

var ErrNodeSSHAccessMethodDuplicate = errors.New("ssh access method already exists for node target")
var ErrNodeNotManageable = errors.New("node is not manageable")

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type NodeSSHAccessMethodCreateInput struct {
	SSHHost          string
	SSHPort          int
	SSHUser          string
	SSHHostKeySHA256 string
	PrivateKey       string
	IsEnabled        bool
	ActorUserID      *string
}
