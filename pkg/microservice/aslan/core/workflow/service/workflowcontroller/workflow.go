package workflowcontroller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"go.uber.org/zap"
)

var approveChannelMap sync.Map
var cancelChannelMap sync.Map

type workflowCtl struct {
	workflowTask  *commonmodels.WorkflowTask
	globalContext *sync.Map
	logger        *zap.SugaredLogger
	ack           func()
}

func NewWorkflowController(workflowTask *commonmodels.WorkflowTask, logger *zap.SugaredLogger) *workflowCtl {
	ctl := &workflowCtl{
		workflowTask: workflowTask,
		logger:       logger,
	}
	ctl.ack = ctl.updateWorkflowTask
	return ctl
}

func CancelWorkflowTask(workflowName string, id int64) error {
	value, ok := cancelChannelMap.Load(fmt.Sprintf("%s-%d", workflowName, id))
	if !ok {
		return fmt.Errorf("no mactched task found, id: %d, workflow name: %s", id, workflowName)
	}
	if f, ok := value.(context.CancelFunc); ok {
		f()
		return nil
	}
	return fmt.Errorf("cancel func type mismatched, id: %d, workflow name: %s", id, workflowName)
}

func (c *workflowCtl) Run(ctx context.Context, concurrency int) {
	c.workflowTask.Status = config.StatusRunning
	c.workflowTask.StartTime = time.Now().Unix()
	c.ack()
	c.logger.Infof("start workflow: %s,status: %s", c.workflowTask.WorkflowName, c.workflowTask.Status)
	defer func() {
		c.workflowTask.EndTime = time.Now().Unix()
		c.logger.Infof("finish workflow: %s,status: %s", c.workflowTask.WorkflowName, c.workflowTask.Status)
		c.ack()
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelChannelMap.Store(fmt.Sprintf("%s-%d", c.workflowTask.WorkflowName, c.workflowTask.TaskID), cancel)
	RunStages(ctx, c.workflowTask.Stages, concurrency, c.globalContext, c.logger, c.ack)
	updateworkflowStatus(c.workflowTask)
}

func updateworkflowStatus(workflow *commonmodels.WorkflowTask) {
	statusMap := map[config.Status]int{
		config.StatusCancelled: 4,
		config.StatusTimeout:   3,
		config.StatusFailed:    2,
		config.StatusPassed:    1,
		config.StatusSkipped:   0,
	}

	// 初始化workflowStatus为创建状态
	workflowStatus := config.StatusRunning

	stageStatus := make([]int, len(workflow.Stages))

	for i, j := range workflow.Stages {
		statusCode, ok := statusMap[j.Status]
		if !ok {
			statusCode = -1
		}
		stageStatus[i] = statusCode
	}
	var workflowStatusCode int
	for i, code := range stageStatus {
		if i == 0 || code > workflowStatusCode {
			workflowStatusCode = code
		}
	}

	for taskstatus, code := range statusMap {
		if workflowStatusCode == code {
			workflowStatus = taskstatus
			break
		}
	}
	workflow.Status = workflowStatus
}

func (c *workflowCtl) updateWorkflowTask() {
	// TODO update workflow task
}
