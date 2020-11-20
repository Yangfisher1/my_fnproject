package models

// StateMachine Reference: https://states-language.net
type StateMachine struct {
	Comment string
	StartAt string
	States  map[string]*State
}

// State Single state in the state machine
type State struct {
	Type     string
	AppName  string
	FuncName string
	Next     string
	Comment  string
	End      bool
}

// Constants
const (
	StateTypeTask = "Task"
)
