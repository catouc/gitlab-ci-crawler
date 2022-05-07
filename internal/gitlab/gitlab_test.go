package gitlab

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type doer struct {
	doFunc func(r *http.Request) (*http.Response, error)
}

func (d *doer) Do(r *http.Request) (*http.Response, error) {
	return d.doFunc(r)
}

func TestClient_GetRawFileFromProject(t *testing.T) {
	testData := []struct {
		Name   string
		DoFunc func(r *http.Request) (*http.Response, error)
		Out    []byte
		Err    error
	}{
		{
			Name: "ValidFile",
			DoFunc: func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("test")),
				}, nil
			},
			Out: []byte{'t', 'e', 's', 't'},
			Err: nil,
		},
		{
			Name: "FileNotFound",
			DoFunc: func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("test")),
				}, nil
			},
			Out: nil,
			Err: &RawFileError{
				Err:       ErrRawFileNotFound,
				Msg:       "failed to get raw file",
				File:      ".gitlab-ci.yml",
				Ref:       "master",
				ProjectID: 1,
			},
		},
		{
			Name: "FileNotFound",
			DoFunc: func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("test")),
				}, nil
			},
			Out: nil,
			Err: &RawFileError{
				Err:       ErrRawFileNotFound,
				Msg:       "failed to get raw file",
				File:      ".gitlab-ci.yml",
				Ref:       "master",
				ProjectID: 1,
			},
		},
	}

	for _, td := range testData {
		t.Run(td.Name, func(t *testing.T) {
			d := doer{
				doFunc: td.DoFunc,
			}
			c := NewClient("https://example.com", "", &d)
			bytes, err := c.GetRawFileFromProject(context.TODO(), 1, ".gitlab-ci.yml", "master")

			if td.Err != nil {
				var re *RawFileError
				assert.ErrorAs(t, err, &re)
			}
			assert.Equal(t, td.Out, bytes)
		})
	}
}
