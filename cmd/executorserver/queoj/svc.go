package queoj

import (
	"context"
	"encoding/json"
	"github.com/criyle/go-judge/cmd/executorserver/queoj/problemclient"
	record_status "github.com/criyle/go-judge/cmd/executorserver/queoj/record-status"
	"github.com/criyle/go-judge/worker"
	"github.com/go-redis/redis"
	"github.com/tal-tech/go-zero/core/discov"
	"github.com/tal-tech/go-zero/core/logx"
	red "github.com/tal-tech/go-zero/core/stores/redis"
	"github.com/tal-tech/go-zero/zrpc"
	"math/rand"
	"time"
)

var redisRecordKey string = "r-in-buf"
var redisRecordBackKey string = "r-back-buf"

type Record struct {
	Id uint64  `gorm:"primaryKey,autoIncrement" json:"id"`
	Uid uint64 `json:"uid"`
	Pid int32 `json:"pid"`
	Time uint64 `json:"time"`
	Language uint32 `json:"language"`
	Status uint32 `json:"status"`
	TimeUsed uint64 `json:"time_used"`
	SpaceUsed uint64 `json:"space_used"`
	Code string `json:"code"`
}

type ServiceContext struct {
	redis *red.Redis
	ProblemClient problemclient.Problem
	ctx context.Context
	stop func()
	maxGoRoutines int
	worker worker.Worker
}

func NewServiceContext(maxGoRoutines int,worker worker.Worker) *ServiceContext {
	ctx , stop := context.WithCancel(context.Background())
	return &ServiceContext{
		redis:         red.New("localhost:6379", red.WithPass("queoj")),
		ProblemClient: problemclient.NewProblem(zrpc.MustNewClient(zrpc.RpcClientConf{
			Etcd:      discov.EtcdConf{
				Hosts:              []string{"localhost:2379"},
				Key:                "problem.rpc",
			},
		})),
		ctx: ctx,
		stop: stop,
		maxGoRoutines: maxGoRoutines,
		worker: worker,
	}
}

func (svc *ServiceContext) Start() {
	for i := 0; i < svc.maxGoRoutines; i++ {
		go func() {
			for {
				select {
				case <-svc.ctx.Done():
					return
				default:{
					recordString, err := svc.redis.Rpop(redisRecordKey)
					if err != nil {
						if err!=redis.Nil {
							logx.Errorf("get from redis error :%v",err)
						}
						time.Sleep(time.Duration(rand.Intn(100) + 950) * time.Millisecond)
						break
					}
					var record Record
					err = json.Unmarshal([]byte(recordString), &record)
					if err != nil {
						logx.Errorf("unmarshal record error :%v",err)
						record.Status = record_status.InternalError
					}else {
						svc.Judge(&record)
					}

					svc.SendResult(&record)
				}
				}
			}
		}()
	}
}


func (svc *ServiceContext) Stop()  {
	svc.stop()
}


func (svc *ServiceContext) Judge(record *Record) {
	switch record.Language {
	case 1:
		svc.submitJava(record)
	case 2:
		svc.submitCpp(record)
	case 3:
		svc.submitGo(record)
	default:
		panic("暂不支持该语言！")
	}
}


func (svc *ServiceContext) SendResult(record *Record) {
	data, err := json.Marshal(record)
	if err != nil {
		logx.Errorf("marshal record error :%v",err)
		return
	}
	svc.redis.Lpush(redisRecordBackKey,string(data))
}
