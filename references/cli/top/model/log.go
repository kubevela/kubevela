/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"context"
	"fmt"
	"regexp"
	"text/template"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/wercker/stern/stern"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/pkg/multicluster"
)

// PrintLogOfPod print the log during 48h of aimed pod
func PrintLogOfPod(ctx context.Context, config *rest.Config, cluster, namespace, podName, containerName string) (chan string, error) {
	if cluster != "local" {
		ctx = multicluster.ContextWithClusterName(ctx, cluster)
	}
	pod, err := regexp.Compile(podName + ".*")
	if err != nil {
		return nil, fmt.Errorf("fail to compile '%s' for logs query", podName+".*")
	}
	container := regexp.MustCompile(".*")
	if containerName != "" {
		container = regexp.MustCompile(containerName + ".*")
	}
	selector := labels.NewSelector()

	logC := make(chan string, 1024)

	go podLog(ctx, config, namespace, pod, container, selector, logC)

	return logC, nil
}

func podLog(ctx context.Context, config *rest.Config, namespace string, pod, container *regexp.Regexp, selector labels.Selector, logC chan string) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		logC <- err.Error()
		return
	}

	added, removed, err := stern.Watch(ctx,
		clientSet.CoreV1().Pods(namespace),
		pod,
		container,
		nil,
		[]stern.ContainerState{stern.RUNNING, stern.TERMINATED},
		selector,
	)
	if err != nil {
		logC <- err.Error()
		return
	}

	tails := make(map[string]*stern.Tail)

	var t string
	if color.NoColor {
		t = "{{.ContainerName}} {{.Message}}"
	} else {
		t = "{{color .ContainerColor .ContainerName}} {{.Message}}"
	}

	funs := map[string]interface{}{
		"color": func(color color.Color, text string) string {
			return color.SprintFunc()(text)
		},
	}
	template, err := template.New("log").Funcs(funs).Parse(t)
	if err != nil {
		if err != nil {
			logC <- errors.Wrap(err, "unable to parse template").Error()
			return
		}
	}

	go func() {
		for p := range added {
			id := p.GetID()
			if tails[id] != nil {
				continue
			}
			// 48h
			dur, _ := time.ParseDuration("48h")
			tail := stern.NewTail(p.Namespace, p.Pod, p.Container, template, &stern.TailOptions{
				Timestamps:   true,
				SinceSeconds: int64(dur.Seconds()),
				Exclude:      nil,
				Include:      nil,
				Namespace:    false,
				TailLines:    nil, // default for all logs
			})
			tails[id] = tail

			tail.Start(ctx, clientSet.CoreV1().Pods(p.Namespace), logC)
		}
	}()

	go func() {
		for p := range removed {
			id := p.GetID()
			if tails[id] == nil {
				continue
			}
			tails[id].Close()
			delete(tails, id)
		}
	}()

	<-ctx.Done()
}
