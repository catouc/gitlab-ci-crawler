package gitlab

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLinkHeader(t *testing.T) {
	testData := []struct {
		Name string
		In   string
		Out  linkHeader
		Err  error
	}{
		{
			Name: "ValidLinkHeader",
			In:   "<https://gitlab.example.com/api/v4/projects?pagination=keyset&per_page=50&order_by=id&sort=asc&id_after=42>; rel=\"next\"",
			Out: linkHeader{
				link: "https://gitlab.example.com/api/v4/projects?pagination=keyset&per_page=50&order_by=id&sort=asc&id_after=42",
				rel:  "next",
			},
			Err: nil,
		},
		{
			Name: "InvalidLinkHeader",
			In:   "https://gitlab.example.com/api/v4/projects?pagination=keyset&per_page=50&order_by=id&sort=asc&id_after=42>; rel=\"next\"",
			Out:  linkHeader{},
			Err:  ErrNoRFC5988LinkHeader,
		},
		{
			Name: "EmptyString",
			In:   "",
			Out:  linkHeader{},
			Err:  ErrNoRFC5988LinkHeader,
		},
		{
			Name: "InvalidURL",
			In:   "<://invalid.com>; rel=\"next\"",
			Out:  linkHeader{},
			Err: &ParseError{
				Err: &url.Error{
					Op:  "Parse",
					URL: "://invalid.com",
					Err: errors.New("missing protocol scheme"),
				},
				Header: "<://invalid.com>; rel=\"next\"",
			},
		},
	}

	for _, td := range testData {
		t.Run(td.Name, func(t *testing.T) {
			lh, err := parseLinkHeader(td.In)

			if td.Err != nil {
				var pe *ParseError
				assert.ErrorAs(t, err, &pe)
				errors.As(err, &pe)
				assert.Equal(t, pe.Header, td.In)
			}
			assert.Equal(t, td.Out, lh)
		})
	}

}

func TestParseLinkHeaders(t *testing.T) {
	testData := []struct {
		Name string
		In   string
		Out  []linkHeader
		Err  error
	}{
		{
			Name: "ValidSingleHeader",
			In:   "<https://example.com>; rel=\"next\"",
			Out: []linkHeader{
				{
					link: "https://example.com",
					rel:  "next",
				},
			},
			Err: nil,
		},
		{
			Name: "ValidMultipleHeaders",
			In:   "<https://example.com>; rel=\"next\",<https://copyright.example.com>; rel=\"copyright\"",
			Out: []linkHeader{
				{
					link: "https://example.com",
					rel:  "next",
				},
				{
					link: "https://copyright.example.com",
					rel:  "copyright",
				},
			},
			Err: nil,
		},
		{
			Name: "InValidMultipleHeaders",
			In:   "<://invalid.com>; rel=\"next\",<https://copyright.example.com>; rel=\"copyright\"",
			Out:  nil,
			Err: &ParseError{
				Header: "<://invalid.com>; rel=\"next\",<https://copyright.example.com>; rel=\"copyright\"",
				Err: &url.Error{
					Op:  "Parse",
					URL: "://invalid.com",
					Err: errors.New("missing protocol scheme"),
				},
			},
		},
		{
			Name: "NoHeaders",
			In:   "",
			Out:  nil,
			Err:  nil,
		},
	}

	for _, td := range testData {
		t.Run(td.Name, func(t *testing.T) {
			lhs, err := parseLinkHeaders(td.In)
			if err != nil {
				var pe *ParseError
				assert.ErrorAs(t, err, &pe)
			}
			assert.Equal(t, td.Out, lhs)
		})
	}
}

func TestGetNextLinkFromLinkHeaders(t *testing.T) {
	testData := []struct {
		Name string
		In   []linkHeader
		Out  linkHeader
	}{
		{
			Name: "NextLinkHeaderSingle",
			In: []linkHeader{
				{
					link: "https://example.com",
					rel:  "next",
				},
			},
			Out: linkHeader{
				link: "https://example.com",
				rel:  "next",
			},
		},
		{
			Name: "NextLinkHeaderMultiple",
			In: []linkHeader{
				{
					link: "https://example.com",
					rel:  "next",
				},
				{
					link: "https://notexample.com",
					rel:  "notnext",
				},
			},
			Out: linkHeader{
				link: "https://example.com",
				rel:  "next",
			},
		},
		{
			Name: "NoNextLinkHeaderMultiple",
			In: []linkHeader{
				{
					link: "https://example.com",
					rel:  "notnext",
				},
				{
					link: "https://notexample.com",
					rel:  "notnext",
				},
			},
			Out: linkHeader{},
		},
		{
			Name: "EmptySlice",
			In:   []linkHeader{},
			Out:  linkHeader{},
		},
	}

	for _, td := range testData {
		t.Run(td.Name, func(t *testing.T) {
			lh := getNextLinkFromLinkHeaders(td.In)
			assert.Equal(t, td.Out, lh)
		})
	}
}
