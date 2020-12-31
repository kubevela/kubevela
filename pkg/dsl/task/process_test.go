package task

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/bmizerany/assert"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"github.com/labstack/echo"
)

const TaskTemplate = `
parameter: {
  serviceURL: string
}

processing: {
  output: {
    token ?: string
  }
  http: {
    method: *"GET" | string
    url: parameter.serviceURL
    request: {
        body ?: bytes
        header: {}
        trailer: {}
    }
  }
}

patch: {
  data: token: processing.output.token
}

output: {
  data: processing.output.token
}
`

func TestContext(t *testing.T) {
	var r cue.Runtime

	lpV := `test: "just a test"`
	inst, err := r.Compile("lp", lpV)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := Context{Obj: inst.Value()}
	val := ctx.Lookup("test")
	assert.Equal(t, true, val.Exists())

	intV := `iTest: 64`
	iInst, err := r.Compile("int", intV)
	if err != nil {
		t.Error(err)
		return
	}
	iCtx := Context{Obj: iInst.Value()}
	iVal := iCtx.Int64("iTest")
	assert.Equal(t, int64(64), iVal)
}

func TestHttpTask(t *testing.T) {
	s, _ := NewMock()
	defer s.Close()

	r := cue.Runtime{}
	taskTemplate, err := r.Compile("", TaskTemplate)
	if err != nil {
		t.Fatal(err)
	}
	taskTemplate, _ = taskTemplate.Fill(map[string]interface{}{
		"serviceURL": fmt.Sprintf("%s/api/v1/token?val=test-token", s.URL),
	}, "parameter")

	inst, err := Process(taskTemplate)
	if err != nil {
		t.Fatal(err)
	}
	output := inst.Lookup("output")
	data, _ := cueJson.Marshal(output)
	fmt.Println(data)
}

func NewMock() (*httptest.Server, *MockServer) {
	mux := echo.New()
	m := &MockServer{}
	mux.GET("/api/v1/token", m.GenToken())
	return httptest.NewServer(mux), m
}

type MockServer struct{}

func (m *MockServer) GenToken() echo.HandlerFunc {
	return func(c echo.Context) error {
		params := c.QueryParams()
		token := params.Get("val")
		return c.JSON(200, map[string]interface{}{
			"token": token,
		})
	}
}
