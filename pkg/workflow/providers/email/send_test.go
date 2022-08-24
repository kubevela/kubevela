/*
Copyright 2021 The KubeVela Authors.

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

package email

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/mock"

	. "github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/gomail.v2"
)

func TestSendEmail(t *testing.T) {
	var dial *gomail.Dialer
	testCases := map[string]struct {
		from        string
		expectedErr error
		errMsg      string
	}{
		"success": {
			from: `
from: {
address: "kubevela@gmail.com"
alias: "kubevela-bot"
password: "pwd"
host: "smtp.test.com"
port: 465
}
to: ["user1@gmail.com", "user2@gmail.com"]
content: {
subject: "Subject"
body: "Test body."
}
stepID: "success"
`,
		},
		"no-step-id": {
			from:        ``,
			expectedErr: errors.New("failed to lookup value: var(path=stepID) not exist"),
		},
		"no-sender": {
			from:        `stepID:"no-sender"`,
			expectedErr: errors.New("failed to lookup value: var(path=from) not exist"),
		},
		"no-receiver": {
			from: `
from: {
address: "kubevela@gmail.com"
alias: "kubevela-bot"
password: "pwd"
host: "smtp.test.com"
port: 465
}
stepID: "no-receiver"
`,
			expectedErr: errors.New("failed to lookup value: var(path=to) not exist"),
		},
		"no-content": {
			from: `
from: {
address: "kubevela@gmail.com"
alias: "kubevela-bot"
password: "pwd"
host: "smtp.test.com"
port: 465
}
to: ["user1@gmail.com", "user2@gmail.com"]
stepID: "no-content"
`,
			expectedErr: errors.New("failed to lookup value: var(path=content) not exist"),
		},
		"send-fail": {
			from: `
from: {
address: "kubevela@gmail.com"
alias: "kubevela-bot"
password: "pwd"
host: "smtp.test.com"
port: 465
}
to: ["user1@gmail.com", "user2@gmail.com"]
content: {
subject: "Subject"
body: "Test body."
}
stepID: "send-fail"
`,
			errMsg: "fail to send",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			patch := ApplyMethod(reflect.TypeOf(dial), "DialAndSend", func(_ *gomail.Dialer, _ ...*gomail.Message) error {
				return nil
			})
			defer patch.Reset()

			act := &mock.Action{}

			if tc.errMsg != "" {
				patch.Reset()
				patch = ApplyMethod(reflect.TypeOf(dial), "DialAndSend", func(_ *gomail.Dialer, _ ...*gomail.Message) error {
					return errors.New(tc.errMsg)
				})
				defer patch.Reset()
			}
			v, err := value.NewValue(tc.from, nil, "")
			r.NoError(err)
			prd := &provider{}
			err = prd.Send(nil, v, act)
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr.Error(), err.Error())
				return
			}
			r.NoError(err)
			r.Equal(act.Phase, "Wait")

			// mock reconcile
			time.Sleep(time.Second)
			err = prd.Send(nil, v, act)
			if tc.errMsg != "" {
				r.Equal(fmt.Errorf("failed to send email: %s", tc.errMsg), err)
				return
			}
			r.NoError(err)
		})
	}
}

func TestInstall(t *testing.T) {
	p := providers.NewProviders()
	Install(p)
	h, ok := p.GetHandler("email", "send")
	r := require.New(t)
	r.Equal(ok, true)
	r.Equal(h != nil, true)
}
