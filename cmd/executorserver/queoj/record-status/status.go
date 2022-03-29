package record_status

const (
	New = 0
	Accept = 1
	MemoryLimitExceeded = 2
	TimeLimitExceeded = 3
	OutputLimitExceeded = 4
	FileError = 5
	NonzeroExitStatus = 6
	Signalled = 7
	InternalError = 8
	CompileError = 9
	WrongAnswer = 10
)
