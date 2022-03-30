package queoj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/cmd/executorserver/queoj/problemclient"
	record_status "github.com/criyle/go-judge/cmd/executorserver/queoj/record-status"
	"github.com/criyle/go-judge/envexec"
	"github.com/tal-tech/go-zero/core/logx"
	"strconv"
)


var CppCompileReq = `
{
    "cmd": [{
        "args": ["/usr/bin/g++", "a.cc" ,"-o","a"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [{
            "content": ""
        }, {
            "name": "stdout",
            "max": 10240
        }, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": 10000000000,
        "memoryLimit": 104857600,
        "procLimit": 50,
        "copyIn": {
            "a.cc": {
                "content": %s
            }
        },
        "copyOut": ["stdout", "stderr"],
        "copyOutCached": ["a.cc", "a"],
        "copyOutDir": "1"
    }]
}
`

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
	//req := model.Request{Cmd: []model.Cmd{{
	//	Args: []string{"/usr/bin/g++", "a.cpp", "-o", "aOut"},
	//	Env:  []string{"PATH=/usr/bin:/bin"},
	//	Files: []*model.CmdFile{
	//		{
	//			Content: &EmptyStr,
	//		},
	//		{
	//			Name: &StdOut,
	//			Max:  &StdOutLimit,
	//		},
	//		{
	//			Name: &StdErr,
	//			Max:  &StdOutLimit,
	//		},
	//	},
	//	CPULimit:    10000000000,
	//	MemoryLimit: 1048576000,
	//	ProcLimit:   50,
	//	CopyIn: map[string]model.CmdFile{
	//		"a.cpp": {
	//			Content: code,
	//		},
	//	},
	//	CopyOut:       []string{"stdout", "stderr"},
	//	CopyOutCached: []string{"a.cpp", "aOut"},
	//	CopyOutDir:    "1",
	//}}}
	var req model.Request
	compileStr := fmt.Sprintf(CppCompileReq, strconv.Quote(*code))
	err := json.Unmarshal([]byte(compileStr), &req)
	if err != nil {
		logx.Error(err)
		return "",err
	}

	marshal, _ := json.Marshal(&req)
	fmt.Println(string(marshal))

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

var CppRunReq = `
{
    "cmd": [{
        "args": ["a"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [{
            "content": "%s"
        }, {
            "name": "stdout",
            "max": 10240
        }, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": %d,
        "memoryLimit": %d,
        "procLimit": 50,
        "strictMemoryLimit": false,
        "copyIn": {
            "a": {
                "fileId": "%s"
            }
        }
    }]
}
`

func (svc *ServiceContext) runCpp(classId, input string, tl, sl uint64) (uint32, *JudgeResult, error) {
	//req := model.Request{Cmd: []model.Cmd{{
	//	Args: []string{"a"},
	//	Env:  []string{"PATH=/usr/bin:/bin"},
	//	Files: []*model.CmdFile{
	//		{
	//			Content: &input,
	//		},
	//		{
	//			Name: &StdOut,
	//			Max:  &StdOutLimit,
	//		},
	//		{
	//			Name: &StdErr,
	//			Max:  &StdOutLimit,
	//		},
	//	},
	//	CPULimit:    tl,
	//	MemoryLimit: sl,
	//	ProcLimit:   50,
	//	CopyIn: map[string]model.CmdFile{
	//		"a": {
	//			FileID: &classId,
	//		},
	//	},
	//}}}

	var req model.Request

	compileStr := fmt.Sprintf(CppRunReq, input, tl,sl,classId)
	err := json.Unmarshal([]byte(compileStr), &req)
	if err != nil {
		logx.Error(err)
		return 0,nil,err
	}

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
