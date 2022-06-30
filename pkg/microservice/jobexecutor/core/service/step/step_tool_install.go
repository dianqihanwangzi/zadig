package step

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/koderover/zadig/pkg/microservice/reaper/config"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/tool/httpclient"

	s3tool "github.com/koderover/zadig/pkg/tool/s3"
	"github.com/koderover/zadig/pkg/types/step"
	"gopkg.in/yaml.v3"
)

type ToolInstallStep struct {
	spec       *step.StepToolInstallSpec
	envs       []string
	secretEnvs []string
	workspace  string
}

func NewToolInstallStep(spec interface{}, workspace string, envs, secretEnvs []string) (*ToolInstallStep, error) {
	toolInstallStep := &ToolInstallStep{workspace: workspace, envs: envs, secretEnvs: secretEnvs}
	yamlBytes, err := yaml.Marshal(spec)
	if err != nil {
		return toolInstallStep, fmt.Errorf("marshal spec %+v failed", spec)
	}
	if err := yaml.Unmarshal(yamlBytes, &toolInstallStep.spec); err != nil {
		return toolInstallStep, fmt.Errorf("unmarshal spec %s to shell spec failed", yamlBytes)
	}
	return toolInstallStep, nil
}

func (s *ToolInstallStep) Run(ctx context.Context) error {
	return s.runIntallationScripts()
}

func (s *ToolInstallStep) runIntallationScripts() error {
	if s.spec == nil {
		return nil
	}
	var (
		openProxy                   bool
		proxyScript, disProxyScript string
	)

	var tmpPath string
	scripts := []string{}
	scripts = append(scripts, "set -ex")

	// 获取用户指定环境变量
	s.envs = append(s.envs, Environs(s.spec.Envs)...)

	if openProxy {
		scripts = append(scripts, proxyScript)
	}

	// 如果应用有配置下载路径
	if s.spec.Download != "" {
		s.spec.S3Storage.Subfolder = fmt.Sprintf("%s/%s-v%s", config.ConstructCachePath, s.spec.Name, s.spec.Version)
		filepath := strings.Split(s.spec.Download, "/")
		fileName := filepath[len(filepath)-1]
		forcedPathStyle := true
		if s.spec.S3Storage.Provider == setting.ProviderSourceAli {
			forcedPathStyle = false
		}

		tmpPath = path.Join(os.TempDir(), fileName)
		s3client, err := s3tool.NewClient(s.spec.S3Storage.Endpoint, s.spec.S3Storage.Ak, s.spec.S3Storage.Sk, s.spec.S3Storage.Insecure, forcedPathStyle)
		if err == nil {
			objectKey := GetObjectPath(fileName, s.spec.S3Storage.Subfolder)
			err = s3client.Download(
				s.spec.S3Storage.Bucket,
				objectKey,
				tmpPath,
			)

			// 缓存不存在
			if err != nil {
				err := httpclient.Download(s.spec.Download, tmpPath)
				if err != nil {
					return err
				}
				s3client.Upload(
					s.spec.S3Storage.Bucket,
					tmpPath,
					objectKey,
				)
				fmt.Printf("Package loaded from url: %s\n", s.spec.Download)
			}
		} else {
			err := httpclient.Download(s.spec.Download, tmpPath)
			if err != nil {
				return err
			}
		}
	}

	for j, command := range s.spec.Scripts {
		realCommand := strings.ReplaceAll(command, config.FilepathParam, tmpPath)
		s.spec.Scripts[j] = realCommand
	}

	scripts = append(scripts, s.spec.Scripts...)

	if openProxy {
		scripts = append(scripts, disProxyScript)
	}
	uid, _ := uuid.NewUUID()
	file := filepath.Join(os.TempDir(), fmt.Sprintf("install_script_%d.sh", uid))
	if err := ioutil.WriteFile(file, []byte(strings.Join(scripts, "\n")), 0700); err != nil {
		return fmt.Errorf("write script file error: %v", err)
	}

	cmd := exec.Command("/bin/bash", file)
	cmd.Dir = s.workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = s.envs

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func Environs(envs []string) []string {
	resp := []string{}
	for _, val := range envs {
		if val == "" {
			continue
		}

		if len(strings.Split(val, "=")) != 2 {
			continue
		}

		replaced := strings.Replace(val, "$HOME", config.Home(), -1)
		resp = append(resp, replaced)
	}
	return resp
}

func GetObjectPath(name, subFolder string) string {
	// target should not be started with /
	if subFolder != "" {
		return strings.TrimLeft(filepath.Join(subFolder, name), "/")
	}

	return strings.TrimLeft(name, "/")
}