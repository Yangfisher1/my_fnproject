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

// BenchmarkRequest - benchmark request
type BenchmarkRequest struct {
	AppName  string
	FuncName string
	Count    uint64
	Time     uint64
}

// BenchmarkResult - benchmark result
type BenchmarkResult struct {
	AverageLatency        float64
	ElapsedTime           int64
	TotalCompletedRequest int64
	Checkpoints           []Checkpoint
	AverageThroughput     float64
	TotalError            int64
}

// Checkpoint - checkpoint
type Checkpoint struct {
	Start            int64
	End              int64
	Checkpoints      []int64
	ErrorCount       int64
	CompletedRequest int64
}
