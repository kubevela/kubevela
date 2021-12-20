package addon

import (
	"embed"
	"encoding/json"
	"github.com/google/go-github/v32/github"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"
)

const (
	// baseURLPath is a non-empty Client.BaseURL path to use during tests,
	// to ensure relative URLs are used for all endpoints. See issue #752.
	baseURLPath = "/api-v3"
)

var (
	//go:embed testdata
	testdata       embed.FS
	testdataPrefix = "testdata"
)

func setup() (client *github.Client, mux *http.ServeMux, teardown func()) {
	// mux is the HTTP request multiplexer used with the test server.
	mux = http.NewServeMux()

	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseURLPath+"/", http.StripPrefix(baseURLPath, mux))

	// server is a test HTTP server used to provide mock API responses.
	server := httptest.NewServer(apiHandler)

	// client is the GitHub client being tested and is
	// configured to use test server.
	client = github.NewClient(nil)
	URL, _ := url.Parse(server.URL + baseURLPath + "/")
	client.BaseURL = URL
	client.UploadURL = URL

	return client, mux, server.Close
}

func TestGitHubReader(t *testing.T) {
	client, mux, teardown := setup()
	githubPattern := "/repos/o/r/contents/"
	mux.HandleFunc(githubPattern, func(rw http.ResponseWriter, req *http.Request) {
		queryPath := strings.TrimPrefix(req.URL.Path, githubPattern)
		localPath := path.Join(testdataPrefix, queryPath)
		file, err := testdata.ReadFile(localPath)
		// test if it's a file
		if err == nil {
			content := &github.RepositoryContent{Type: String("file"), Name: String(path.Base(queryPath)), Size: Int(len(file)), Encoding: String(""), Path: String(queryPath), Content: String(string(file))}
			res, _ := json.Marshal(content)
			rw.Write(res)
		}

		// otherwise, it could be directory
		dir, err := testdata.ReadDir(localPath)
		if err == nil {
			contents := make([]*github.RepositoryContent, 0)
			for _, item := range dir {
				tp := "file"
				if item.IsDir() {
					tp = "dir"
				}
				contents = append(contents, &github.RepositoryContent{Type: String(tp), Name: String(item.Name()), Path: String(path.Join(queryPath, item.Name()))})
			}
			dRes, _ := json.Marshal(contents)
			rw.Write(dRes)
		}

		rw.Write([]byte("invalid github query"))
	})
	defer teardown()

	gith := &gitHelper{
		Client: client,
		Meta: &utils.Content{GithubContent: utils.GithubContent{
			Owner: "o",
			Repo:  "r",
		}},
	}
	var r AsyncReader = &gitReader{gith}
	_, err := r.ReadFile("example/metadata.yaml")
	assert.NoError(t, err)

	testReaderFunc(t, r)
}

var gitHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
	content := &github.RepositoryContent{Type: String("file"), Name: String("LICENSE"), Size: Int(20678), Encoding: String("base64"), Path: String("LICENSE")}
	res, _ := json.Marshal(content)
	rw.Write(res)
}

// Int is a helper routine that allocates a new int value
// to store v and returns a pointer to it.
func Int(v int) *int { return &v }

// String is a helper routine that allocates a new string value
// to store v and returns a pointer to it.
func String(v string) *string { return &v }
