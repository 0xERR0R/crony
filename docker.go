package main

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"io"
	"strings"
	"time"
)

const (
	mailPolicyLabel = "crony.mail_policy"
	cronStringLabel = "crony.schedule"
)

type DockerClient struct {
	cli *client.Client
	evt context.CancelFunc
}

func NewDockerClient() *DockerClient {
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("can't create docker client: ", err)
	}
	return &DockerClient{cli: cli}
}

func (d *DockerClient) ShutDown() {
	_ = d.cli.Close()
	if d.evt != nil {
		d.evt()
	}
}

type OnContainerEvent func(containerId string, containerName string)

func (d *DockerClient) RegisterDockerEventListeners(createFn OnContainerEvent, destroyFn OnContainerEvent) {
	cxt, cancel := context.WithCancel(context.Background())
	d.evt = cancel
	go func() {
		filter := filters.NewArgs()
		filter.Add("type", "container")
		filter.Add("event", "create")
		filter.Add("event", "destroy")
		msg, errChan := d.cli.Events(cxt, types.EventsOptions{
			Filters: filter,
		})
		for {
			select {
			case err := <-errChan:
				log.Error("got error on listening for new docker events: ", err)
			case msg := <-msg:
				containerName := msg.Actor.Attributes["name"]
				containerId := msg.Actor.ID
				log.Infof("received event: '%s' from '%s'", msg.Action, containerName)
				switch msg.Action {
				case "create":
					createFn(containerId, containerName)
				case "destroy":
					destroyFn(containerId, containerName)
				}
			}
		}
	}()
}

type CronyContainer struct {
	ID, Name, CronString, MailPolicy string
}

func (d *DockerClient) GetCronyContainers(containerId string) ([]CronyContainer, error) {
	filterArgs := filters.NewArgs()

	filterArgs.Add("label", cronStringLabel)
	if containerId != "" {
		filterArgs.Add("id", containerId)
	}
	containerList, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err == nil {
		var result []CronyContainer
		for _, container := range containerList {
			result = append(result, CronyContainer{
				ID:         container.ID,
				Name:       strings.Trim(container.Names[0], "/"),
				CronString: strings.Trim(container.Labels[cronStringLabel], "\""),
				MailPolicy: container.Labels[mailPolicyLabel],
			})
		}
		return result, nil
	}
	return nil, err
}

func (d *DockerClient) ContainerWait(name string) (int64, error) {
	return d.cli.ContainerWait(context.Background(), name)
}

func (d *DockerClient) ContainerLogs(name string, startTime time.Time) (io.ReadCloser, error) {
	return d.cli.ContainerLogs(context.Background(), name, types.ContainerLogsOptions{ShowStdout: true,
		ShowStderr: true,
		Since:      startTime.Format("2006-01-02T15:04:05")})

}

func (d *DockerClient) ContainerStart(name string) error {
	return d.cli.ContainerStart(context.Background(), name, types.ContainerStartOptions{})
}
