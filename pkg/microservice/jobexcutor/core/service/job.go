package job

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/koderover/zadig/pkg/microservice/jobexcutor/config"
	"github.com/koderover/zadig/pkg/microservice/jobexcutor/core/service/meta"
	"github.com/koderover/zadig/pkg/microservice/jobexcutor/core/service/step"
	"gopkg.in/yaml.v3"
)

type Job struct {
	Ctx             *meta.JobContext
	StartTime       time.Time
	ActiveWorkspace string
	UserEnvs        map[string]string
}

func NewJob() (*Job, error) {
	context, err := ioutil.ReadFile(config.JobConfigFile())
	if err != nil {
		return nil, fmt.Errorf("read job config file error: %v", err)
	}

	var ctx *meta.JobContext
	if err := yaml.Unmarshal(context, &ctx); err != nil {
		return nil, fmt.Errorf("cannot unmarshal job data: %v", err)
	}

	ctx.Paths = config.Path()

	job := &Job{
		Ctx: ctx,
	}

	err = job.EnsureActiveWorkspace(ctx.Workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure active workspace `%s`: %s", ctx.Workspace, err)
	}

	userEnvs := job.getUserEnvs()
	job.UserEnvs = make(map[string]string, len(userEnvs))
	for _, env := range userEnvs {
		items := strings.Split(env, "=")
		if len(items) != 2 {
			continue
		}

		job.UserEnvs[items[0]] = items[1]
	}

	return job, nil
}

func (j *Job) EnsureActiveWorkspace(workspace string) error {
	if workspace == "" {
		tempWorkspace, err := ioutil.TempDir(os.TempDir(), "reaper")
		if err != nil {
			return fmt.Errorf("create workspace error: %v", err)
		}
		j.ActiveWorkspace = tempWorkspace
		return os.Chdir(j.ActiveWorkspace)
	}

	err := os.MkdirAll(workspace, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %v", err)
	}
	j.ActiveWorkspace = workspace

	return os.Chdir(j.ActiveWorkspace)
}

func (j *Job) getUserEnvs() []string {
	envs := []string{
		"CI=true",
		"ZADIG=true",
		fmt.Sprintf("HOME=%s", config.Home()),
		fmt.Sprintf("WORKSPACE=%s", j.ActiveWorkspace),
	}

	j.Ctx.Paths = strings.Replace(j.Ctx.Paths, "$HOME", config.Home(), -1)
	envs = append(envs, fmt.Sprintf("PATH=%s", j.Ctx.Paths))
	envs = append(envs, fmt.Sprintf("DOCKER_HOST=%s", config.DockerHost()))
	envs = append(envs, j.Ctx.Envs...)
	envs = append(envs, j.Ctx.SecretEnvs...)

	return envs
}

func (j *Job) Run(ctx context.Context) error {
	for _, stepInfo := range j.Ctx.Steps {
		var stepInstance step.Step
		var err error
		switch stepInfo.StepType {
		case "shell":
			stepInstance, err = step.NewShellStep(ctx, j.ActiveWorkspace, j.getUserEnvs(), j.Ctx.SecretEnvs)
			if err != nil {
				return err
			}
		}
		if err := stepInstance.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}
