package linuxcontainer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/rlimit"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.Environment = &environ{}

// environ defines interface to access container resources
type environ struct {
	container.Environment
	cgPool  CgroupPool
	wd      *os.File // container work dir
	cpuset  string
	seccomp []syscall.SockFilter
	cpuRate bool
}

// Destroy destories the environment
func (c *environ) Destroy() error {
	return c.Environment.Destroy()
}

func (c *environ) Reset() error {
	return c.Environment.Reset()
}

// Execve execute process inside the environment
func (c *environ) Execve(ctx context.Context, param envexec.ExecveParam) (envexec.Process, error) {
	var (
		cg       Cgroup
		syncFunc func(int) error
		err      error
	)

	limit := param.Limit
	if c.cgPool != nil {
		cg, err = c.cgPool.Get()
		if err != nil {
			return nil, fmt.Errorf("execve: failed to get cgroup %v", err)
		}
		if err := c.setCgroupLimit(cg, limit); err != nil {
			return nil, err
		}
		syncFunc = cg.AddProc
	}

	rLimits := rlimit.RLimits{
		CPU:         uint64(limit.Time.Truncate(time.Second)/time.Second) + 1,
		FileSize:    limit.Output.Byte(),
		Stack:       limit.Stack.Byte(),
		OpenFile:    limit.OpenFile,
		DisableCore: true,
	}

	if limit.StrictMemory || c.cgPool == nil {
		rLimits.Data = limit.Memory.Byte()
	}

	// wait for sync or error before turn (avoid file close before pass to child process)
	syncDone := make(chan struct{})

	p := container.ExecveParam{
		Args:     param.Args,
		Env:      param.Env,
		Files:    param.Files,
		CTTY:     param.TTY,
		ExecFile: param.ExecFile,
		RLimits:  rLimits.PrepareRLimit(),
		Seccomp:  c.seccomp,
		SyncFunc: func(pid int) error {
			defer close(syncDone)
			if syncFunc != nil {
				return syncFunc(pid)
			}
			return nil
		},
	}
	proc := newProcess(func() runner.Result {
		return c.Environment.Execve(ctx, p)
	}, cg, c.cgPool)

	select {
	case <-proc.done:
	case <-syncDone:
	}

	return proc, nil
}

// WorkDir returns opened work directory, should not close after
func (c *environ) WorkDir() *os.File {
	c.wd.Seek(0, 0)
	return c.wd
}

// Open opens file relative to work directory
func (c *environ) Open(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Openat(int(c.wd.Fd()), path, flags|syscall.O_CLOEXEC, uint32(perm))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		return nil, fmt.Errorf("openAtWorkDir: failed to NewFile")
	}
	return f, nil
}

func (c *environ) setCgroupLimit(cg Cgroup, limit envexec.Limit) error {
	cpuSet := limit.CPUSet
	if cpuSet == "" {
		cpuSet = c.cpuset
	}
	if cpuSet != "" {
		if err := cg.SetCpuset(cpuSet); isCgroupSetHasError(err) {
			return fmt.Errorf("execve: cgroup failed to set cpu_set limit %v", err)
		}
	}
	if c.cpuRate && limit.Rate > 0 {
		if err := cg.SetCPURate(limit.Rate); isCgroupSetHasError(err) {
			return fmt.Errorf("execve: cgroup failed to set cpu_rate limit %v", err)
		}
	}
	if err := cg.SetMemoryLimit(limit.Memory); isCgroupSetHasError(err) {
		return fmt.Errorf("execve: cgroup failed to set memory limit %v", err)
	}
	if err := cg.SetProcLimit(limit.Proc); isCgroupSetHasError(err) {
		return fmt.Errorf("execve: cgroup failed to set process limit %v", err)
	}
	return nil
}

func isCgroupSetHasError(err error) bool {
	return err != nil && !errors.Is(err, cgroup.ErrNotInitialized) && !errors.Is(err, os.ErrNotExist)
}
