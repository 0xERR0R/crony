package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/hako/durafmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

type MailPolicy uint

const (
	Never MailPolicy = iota
	Always
	OnError
)

//nolint:gochecknoglobals // immutable lookup table
var MailPolicyToString = map[MailPolicy]string{
	Never:   "NEVER",
	Always:  "ALWAYS",
	OnError: "ONERROR",
}

func (m MailPolicy) String() string {
	return MailPolicyToString[m]
}

func (m *MailPolicy) Decode(value string) error {
	for k, v := range MailPolicyToString {
		if v == strings.ToUpper(value) {
			*m = k

			return nil
		}
	}

	return fmt.Errorf("unknown value '%s' for MailPolicy, please use one of 'never, always, onerror'", value)
}

type MailConfig struct {
	SmtpHost     string     `envconfig:"smtp_host"     required:"true"`
	SmtpPort     int        `envconfig:"smtp_port"     required:"true"`
	SmtpUser     string     `envconfig:"smtp_user"`
	SmtpPassword string     `envconfig:"smtp_password"`
	MailTo       string     `envconfig:"mail_to"       required:"true"`
	MailFrom     string     `envconfig:"mail_from"     required:"true"`
	MailPolicy   MailPolicy `default:"never"           envconfig:"mail_policy"`
}

func (mc *MailConfig) Validate() error {
	if (mc.SmtpUser != "" && mc.SmtpPassword == "") || (mc.SmtpUser == "" && mc.SmtpPassword != "") {
		return errors.New("SMTP_USER and SMTP_PASSWORD must be provided together, or not at all")
	}

	return nil
}

func (m MailConfig) String() string {
	return fmt.Sprintf(
		"mail config [host=%s, port=%d, user=%s, mailTo=%s, mailFrom=%s, mailPolicy=%s]",
		m.SmtpHost, m.SmtpPort, m.SmtpUser, m.MailTo, m.MailFrom, m.MailPolicy,
	)
}

type MailParams struct {
	ContainerName string
	ReturnCode    int64
	Duration      time.Duration
	StdOut        string
	StdErr        string
}

func (mp MailParams) ShortDuration() string {
	return durafmt.Parse(mp.Duration.Truncate(time.Second)).String()
}

func newTemplate() *template.Template {
	//nolint:staticcheck // ST1018: unicode glyphs in template body are intentional
	return template.Must(template.New("mail-body").Parse(`
		<p>
			📦 Container: ​<b>{{.ContainerName}}</b>,
			Execution: return code 🗠<b>{{.ReturnCode}}</b> in ​⏱️ <b>{{.ShortDuration}}</b>​,
		</p>
			📝 stdOut: ​<pre>{{.StdOut}}</pre>​
			📝 stdErr: ​<pre style="color: #a13d3d">{{.StdErr}}</pre>​
  `))
}

func createTopic(params MailParams) string {
	if params.ReturnCode == 0 {
		return fmt.Sprintf("[SUCCESS] ✔️ '%s' finished in %s", params.ContainerName, params.ShortDuration())
	}

	return fmt.Sprintf("[FAIL] ❌ '%s' failed in %s", params.ContainerName, params.ShortDuration())
}

func SendMail(config *MailConfig, params MailParams) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", config.MailFrom)
	msg.SetHeader("To", config.MailTo)
	msg.SetHeader("Subject", createTopic(params))
	buf := bytes.NewBuffer(nil)
	err := newTemplate().Execute(buf, params)
	if err != nil {
		log.Error("error during template processing", err)
	}
	msg.SetBody("text/html", buf.String())

	d := gomail.NewDialer(config.SmtpHost, config.SmtpPort, config.SmtpUser, config.SmtpPassword)
	if err := d.DialAndSend(msg); err != nil {
		return err
	}

	return nil
}
