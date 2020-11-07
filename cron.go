package main

import (
	"github.com/armon/circbuf"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"time"
)

const (
	maxLogSize = 1 * 1024 * 1024
)

type ContainerJob struct {
	docker        *DockerClient
	containerName string
	mailConfig    *MailConfig
}

func (cj *ContainerJob) Run() {
	log.Infof("starting execution of container '%s'", cj.containerName)
	startTime := time.Now()

	// TODO check container state
	err := cj.docker.ContainerStart(cj.containerName)
	if err != nil {
		log.Errorf("can't start container '%s': ", cj.containerName, err)
		return
	}

	returnCode, err := cj.docker.ContainerWait(cj.containerName)
	if err != nil {
		log.Errorf("can't wait for the end of the execution of container '%s': ", cj.containerName, err)
		return
	}
	endTime := time.Now()

	log.StandardLogger().Logf(logLevelForReturnCode(returnCode), "Execution of container '%s' finished with return code %d", cj.containerName, returnCode)

	out, err := cj.docker.ContainerLogs(cj.containerName, startTime)
	if err != nil {
		log.Errorf("can't retrieve logs for container '%s': ", cj.containerName, err)
		return
	}

	log.Debug("using mail config: ", cj.mailConfig)

	stdOutBuf, _ := circbuf.NewBuffer(maxLogSize)
	stdErrBuf, _ := circbuf.NewBuffer(maxLogSize)
	_, err = stdcopy.StdCopy(stdOutBuf, stdErrBuf, out)
	if err != nil {
		log.Error("can't retrieve output streams: ", err)
	}

	if cj.mailConfig.MailPolicy == Always || (cj.mailConfig.MailPolicy == OnError && returnCode != 0) {
		err = SendMail(cj.mailConfig, MailParams{
			ContainerName: cj.containerName,
			ReturnCode:    returnCode,
			Duration:      endTime.Sub(startTime),
			StdOut:        stdOutBuf.String(),
			StdErr:        stdErrBuf.String(),
		})
		if err != nil {
			log.Error("can't send mail", err)
		}
	}
}

func logLevelForReturnCode(returnCode int64) log.Level {
	if returnCode != 0 {
		return log.WarnLevel
	}
	return log.InfoLevel
}

type SkipLogger struct {
	containerName string
}

func (l *SkipLogger) Info(_ string, _ ...interface{}) {
	log.StandardLogger().Infof("skipping execution of container '%s', is still running", l.containerName)
}

func (l *SkipLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	log.StandardLogger().Error(err, msg, keysAndValues)
}

func createAndStartCron() *cron.Cron {
	c := cron.New()
	c.Start()
	return c
}
