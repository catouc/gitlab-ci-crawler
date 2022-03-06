package gitlab

import (
	"errors"
	"net/url"
	"strings"
)

type linkHeader struct {
	link string
	rel  string
}

func getNextLinkFromLinkHeaders(headers []linkHeader) linkHeader {
	var nextLinkHeader linkHeader

	for _, lh := range headers {
		if lh.rel == "next" {
			nextLinkHeader = lh
		}
	}

	return nextLinkHeader
}

// parseLinkHeaders tries to parse a list of RFC8288 compliant headers
func parseLinkHeaders(headers string) ([]linkHeader, error) {
	var linkHeaders []linkHeader

	links := strings.Split(headers, ",")
	for _, l := range links {
		link, err := parseLinkHeader(strings.Trim(l, " "))
		if err != nil {
			return nil, err
		}

		linkHeaders = append(linkHeaders, link)
	}

	return linkHeaders, nil
}

type ParseError struct {
	Header string
	Err    error
}

func (pe *ParseError) Error() string {
	return "parse error " + pe.Header + ":" + pe.Err.Error()
}

func (pe *ParseError) Unwrap() error {
	return pe.Err
}

var ErrNoRFC5988LinkHeader = errors.New("given string is not valid under RFC5988")

// parseLinkHeader is an incomplete parser for RFC8288 compliant header fields.
// It makes heavy assumptions around how GitLab uses web linking for keyset pagination.
// See https://docs.gitlab.com/ee/api/projects.html#list-all-projects for more info.
func parseLinkHeader(header string) (linkHeader, error) {
	var lh linkHeader

	elems := strings.Split(header, ";")

	if !strings.HasPrefix(elems[0], "<") {
		return linkHeader{}, &ParseError{Err: ErrNoRFC5988LinkHeader, Header: header}
	}

	lh.link = elems[0][1 : len(elems[0])-1]

	_, err := url.Parse(lh.link)
	if err != nil {
		return linkHeader{}, &ParseError{Err: err, Header: header}
	}

	if !strings.HasPrefix(strings.TrimSpace(elems[1]), "rel") {
		return linkHeader{}, &ParseError{Err: ErrNoRFC5988LinkHeader, Header: header}
	}

	trimmedRel := strings.TrimPrefix(strings.TrimSpace(elems[1]), "rel=")

	lh.rel = trimmedRel[1 : len(trimmedRel)-1]
	return lh, nil
}
