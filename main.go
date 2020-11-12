package main

import (
	"context"
	"fmt"
	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configureLogging()

	log.Info("starting crony...")

	c := createAndStartCron()

	dockerClient := NewDockerClient()
	crony := Crony{
		docker:             dockerClient,
		cron:               c,
		containerIdToJobId: make(map[string]cron.EntryID),
	}

	crony.registerContainers()

	dockerClient.RegisterDockerEventListeners(crony.onContainerCreated, crony.onContainerDestroyed)

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%v", 8080),
		Handler: router,
	}

	signals := make(chan os.Signal)
	done := make(chan bool)

	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		log.Infof("Terminating...")

		c.Stop()
		log.Info("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	log.Info("Server is ready to handle requests at :", 8080)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %d: %v\n", 8080, err)
	}

	<-done
	log.Infof("bye...")
}

type Crony struct {
	docker             *DockerClient
	cron               *cron.Cron
	containerIdToJobId map[string]cron.EntryID
}

func (c *Crony) onContainerCreated(containerId string, _ string) {
	containers, err := c.docker.GetCronyContainers(containerId)
	if err != nil {
		log.Fatalf("can't list containers: ", err)
	}

	if len(containers) == 1 {
		c.registerContainer(containers[0])
	}
}

func (c *Crony) onContainerDestroyed(containerId string, containerName string) {
	if jobId, ok := c.containerIdToJobId[containerId]; ok {
		log.Infof("managed container '%s' was stopped, removing cron job", containerName)
		c.cron.Remove(jobId)
		delete(c.containerIdToJobId, containerId)
	}
}

func mailConfig(container CronyContainer) *MailConfig {
	var mailCfg MailConfig
	err := envconfig.Process("crony", &mailCfg)
	if err != nil {
		log.Error("can't parse mail config", err)
		return nil
	}

	if container.MailPolicy != "" {
		var jobMailPolicy MailPolicy
		err := jobMailPolicy.Decode(container.MailPolicy)
		if err != nil {
			log.Error("can't parse job mail policy ", err)
		} else {
			mailCfg.MailPolicy = jobMailPolicy
		}
	}
	return &mailCfg
}

func (c *Crony) registerContainer(container CronyContainer) {
	// TODO check restart policy

	log.Infof("... found managed container '%s'", container.Name)

	log.Infof("... registering container with '%s'", container.CronString)

	job := cron.NewChain(cron.SkipIfStillRunning(&SkipLogger{containerName: container.Name})).Then(&ContainerJob{
		docker:        c.docker,
		containerName: container.Name,
		mailConfig:    mailConfig(container),
	})
	id, err := c.cron.AddJob(container.CronString, job)
	if err != nil {
		log.Fatal("can't register job ", err)
	}

	c.containerIdToJobId[container.ID] = id
}

func (c *Crony) registerContainers() {
	log.Info("starting container registration")
	containers, err := c.docker.GetCronyContainers("")
	if err != nil {
		log.Fatalf("can't list containers: ", err)
	}
	for _, container := range containers {
		c.registerContainer(container)
	}
	log.Info("container registration finished")
}

func configureLogging() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
}
