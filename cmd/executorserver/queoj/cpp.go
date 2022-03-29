package queoj

import (
	"context"
	"errors"
	"fmt"
	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/cmd/executorserver/queoj/problemclient"
	record_status "github.com/criyle/go-judge/cmd/executorserver/queoj/record-status"
	"github.com/criyle/go-judge/envexec"
	"github.com/tal-tech/go-zero/core/logx"
)

func (svc *ServiceContext) submitCpp(record *Record) {
	// 编译
	classId, err := svc.compileCpp(&record.Code)
	if err != nil {
		logx.Error(err)
		record.Status = record_status.CompileError
		return
	}

	io, err := svc.ProblemClient.GetProblemIO(context.Background(), &problemclient.Integer{Value: record.Pid})
	if err != nil {
		logx.Error(err)
		record.Status = record_status.InternalError
		return
	}

	detail, err := svc.ProblemClient.GetProblemDetail(context.Background(), &problemclient.Integer{Value: record.Pid})
	if err != nil {
		return
	}
	status, result, err := svc.runCpp(classId, io.InTxt, detail.TimeLimit, detail.SpaceLimit)
	if err != nil {
		logx.Error(err)
		record.Status = record_status.InternalError
		return
	}
	if status != 1 {
		record.Status = status
		return
	}

	if result.output != io.OutTxt {
		logx.Infof("输出：'%s' , 答案：'%s'", result.output, io.OutTxt)
		record.Status = record_status.WrongAnswer
		return
	} else {
		record.Status = record_status.Accept
		record.TimeUsed = result.timeUsed
		record.SpaceUsed = result.spaceUsed
	}

	logx.Info(fmt.Sprintf("judge cpp {%d} success .", record.Id))
}

func (svc *ServiceContext) compileCpp(code *string) (string, error) {
	req := model.Request{Cmd: []model.Cmd{{
		Args: []string{"/usr/bin/g++", "a.cpp", "-o", "aOut"},
		Env:  []string{"PATH=/usr/bin:/bin"},
		Files: []*model.CmdFile{
			{
				Content: &EmptyStr,
			},
			{
				Name: &StdOut,
				Max:  &StdOutLimit,
			},
			{
				Name: &StdErr,
				Max:  &StdOutLimit,
			},
		},
		CPULimit:    10000000000,
		MemoryLimit: 1048576000,
		ProcLimit:   50,
		CopyIn: map[string]model.CmdFile{
			"a.cpp": {
				Content: code,
			},
		},
		CopyOut:       []string{"stdout", "stderr"},
		CopyOutCached: []string{"a.cpp", "aOut"},
		CopyOutDir:    "1",
	}}}

	request, err := model.ConvertRequest(&req, "")
	if err != nil {
		logx.Error(err)
		return "", err
	}
	rtCh, _ := svc.worker.Submit(context.Background(), request)
	rt := <-rtCh
	logx.Infof("response : %+v", rt)
	if rt.Error != nil {
		return "", err
	}
	status := rt.Results[0].Status
	if status != envexec.StatusAccepted {
		return "", errors.New("编译错误")
	}

	return rt.Results[0].FileIDs["Main.class"], nil
}

func (svc *ServiceContext) runCpp(classId, input string, tl, sl uint64) (uint32, *JudgeResult, error) {
	req := model.Request{Cmd: []model.Cmd{{
		Args: []string{"aOut"},
		Env:  []string{"PATH=/usr/bin:/bin"},
		Files: []*model.CmdFile{
			{
				Content: &input,
			},
			{
				Name: &StdOut,
				Max:  &StdOutLimit,
			},
			{
				Name: &StdErr,
				Max:  &StdOutLimit,
			},
		},
		CPULimit:    tl,
		MemoryLimit: sl,
		ProcLimit:   50,
		CopyIn: map[string]model.CmdFile{
			"aOut": {
				FileID: &classId,
			},
		},
	}}}

	request, err := model.ConvertRequest(&req, "")
	if err != nil {
		logx.Error(err)
		return 0, nil, err
	}
	rtCh, _ := svc.worker.Submit(context.Background(), request)
	rt := <-rtCh
	logx.Infof("response : %+v", rt)
	if rt.Error != nil {
		return 0, nil, err
	}
	status := rt.Results[0].Status
	if status != envexec.StatusAccepted {
		switch status {
		case envexec.StatusInternalError:
			return record_status.InternalError, nil, nil
		case envexec.StatusMemoryLimitExceeded:
			return record_status.MemoryLimitExceeded, nil, nil
		case envexec.StatusTimeLimitExceeded:
			return record_status.TimeLimitExceeded, nil, nil
		case envexec.StatusFileError:
			return record_status.FileError, nil, nil
		case envexec.StatusNonzeroExitStatus:
			return record_status.NonzeroExitStatus, nil, nil
		default:
			return record_status.Signalled, nil, nil
		}
	}

	response, err := model.ConvertResponse(rt, true)
	if err != nil {
		logx.Error(err)
	}
	out := response.Results[0].Files["stdout"]
	tu := uint64(rt.Results[0].Time)
	su := uint64(rt.Results[0].Memory)

	return record_status.Accept, &JudgeResult{
		output:    out,
		timeUsed:  tu,
		spaceUsed: su,
	}, nil
}
