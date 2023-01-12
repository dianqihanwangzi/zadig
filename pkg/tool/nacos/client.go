/*
Copyright 2023 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nacos

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/imroc/req/v3"
	"github.com/koderover/zadig/pkg/tool/httpclient"
	"github.com/koderover/zadig/pkg/types"
	"github.com/pkg/errors"
)

type Client struct {
	*httpclient.Client
	serverAddr string
	UserName   string
	Password   string
	token      string
}

type loginResp struct {
	AccessToken string `json:"accessToken"`
	TokenTtl    int64  `json:"tokenTtl"`
	GlobalAdmin bool   `json:"globalAdmin"`
}

type config struct {
	ID      string `json:"id"`
	DataID  string `json:"dataId"`
	Group   string `json:"group"`
	Content string `json:"content"`
	Format  string `json:"type"`
}

type configResp struct {
	PageItems []*config `json:"pageItems"`
}

type namespace struct {
	NamespaceID   string `json:"namespace"`
	NamespaceName string `json:"namespaceShowName"`
}
type namespaceResp struct {
	Data []*namespace `json:"data"`
}

func NewNacosClient(serverAddr, userName, password string) (*Client, error) {
	host, err := url.Parse(serverAddr)
	if err != nil {
		return nil, errors.Wrap(err, "parse nacos server address failed")
	}
	// add default context path
	if host.Path == "" {
		serverAddr, _ = url.JoinPath(serverAddr, "nacos")
	}
	loginURL, _ := url.JoinPath(serverAddr, "v1/auth/login")
	var result loginResp
	resp, err := req.R().AddQueryParam("username", userName).
		AddQueryParam("password", password).
		SetResult(&result).
		Post(loginURL)
	if err != nil {
		return nil, errors.New("login nacos failed")
	}
	if !resp.IsSuccess() {
		return nil, errors.New("login nacos failed")
	}

	c := httpclient.New(
		httpclient.SetClientHeader("accessToken", result.AccessToken),
		httpclient.SetHostURL(serverAddr),
	)

	return &Client{
		Client:     c,
		serverAddr: serverAddr,
		token:      result.AccessToken,
		UserName:   userName,
		Password:   password,
	}, nil
}

func (c *Client) ListNamespaces() ([]*types.NacosNamespace, error) {
	url := "/v1/console/namespaces"
	res := &namespaceResp{}
	if _, err := c.Client.Get(url, httpclient.SetResult(res)); err != nil {
		return nil, errors.Wrap(err, "list nacos namespace failed")
	}
	resp := []*types.NacosNamespace{}
	for _, namespace := range res.Data {
		resp = append(resp, &types.NacosNamespace{
			NamespaceID:    namespace.NamespaceID,
			NamespacedName: namespace.NamespaceName,
		})
	}
	return resp, nil
}

func (c *Client) ListConfigs(namespaceID string) ([]*types.NacosConfig, error) {
	url := "/v1/cs/configs"
	resp := []*types.NacosConfig{}
	pageNum := 1
	pageSize := 20
	end := false
	for !end {
		res := &configResp{}
		numString := strconv.Itoa(pageNum)
		sizeString := strconv.Itoa(pageSize)
		params := httpclient.SetQueryParams(map[string]string{
			"dataId":   "",
			"group":    "",
			"search":   "accurate",
			"pageNo":   numString,
			"pageSize": sizeString,
			"tenant":   namespaceID,
		})
		if _, err := c.Client.Get(url, params, httpclient.SetResult(res)); err != nil {
			return nil, errors.Wrap(err, "list nacos config failed")
		}
		for _, conf := range res.PageItems {
			resp = append(resp, &types.NacosConfig{
				DataID:  conf.DataID,
				Group:   conf.Group,
				Format:  getFormat(conf.Format),
				Content: conf.Content,
			})
		}
		if len(res.PageItems) < pageSize {
			end = true
		}
	}
	return resp, nil
}

func (c *Client) GetConfig(dataID, group, namespaceID string) (*types.NacosConfig, error) {
	url := "/v1/cs/configs"
	res := &config{}
	params := httpclient.SetQueryParams(map[string]string{
		"dataId": dataID,
		"group":  group,
		"tenant": namespaceID,
		"show":   "all",
	})
	if _, err := c.Client.Get(url, params, httpclient.SetResult(res)); err != nil {
		return nil, errors.Wrap(err, "list nacos config failed")
	}
	return &types.NacosConfig{
		DataID:  res.DataID,
		Group:   res.Group,
		Format:  getFormat(res.Format),
		Content: res.Content,
	}, nil
}

func (c *Client) UpdateConfig(dataID, group, namespaceID, content string) error {
	path := "/v1/cs/configs"
	formValues := map[string]string{
		"dataId":  dataID,
		"group":   group,
		"tenant":  namespaceID,
		"content": content,
	}
	if _, err := c.Client.Post(path, httpclient.SetFromData(formValues)); err != nil {
		return errors.Wrap(err, "update nacos config failed")
	}
	return nil
}

func getFormat(format string) string {
	switch strings.ToLower(format) {
	case "yaml":
		return "YAML"
	case "json":
		return "JSON"
	case "properties":
		return "Properties"
	case "xml":
		return "XML"
	case "html":
		return "HTML"
	default:
		return "text"
	}
}
