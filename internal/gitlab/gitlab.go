package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	gitLabPrivateTokenHeader = "PRIVATE-TOKEN"
	gitLabAPIPath            = "api/v4"
)

// Client holds all authentication and setup information for
// the GitLab connection.
type Client struct {
	Host     string
	Token    string
	HTTPDoer HTTPDoer
}

type HTTPDoer interface {
	Do(r *http.Request) (*http.Response, error)
}

// Project is a minimalist representation of a GitLab project
// from https://docs.gitlab.com/ee/api/projects.html#get-single-project
type Project struct {
	ID                int    `json:"id"`
	DefaultBranch     string `json:"default_branch"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// NewClient sets up a client struct for all relevant GitLab auth
// you can give it a custom http.Client as well for things like
// timeouts.
func NewClient(host, token string, httpDoer HTTPDoer) *Client {

	return &Client{
		Host:     host,
		Token:    token,
		HTTPDoer: httpDoer,
	}
}

// StreamAllProjects iterates through all projects in a GitLab and streams them in batches of pageSize
// into the projectsChan. Due to this you want to buffer the projectsChan channel to something like 2 x pageSize
// depending on the speed and complexity of your consuming function.
// The authentication check retries for max 30s using an exponential backoff but will exit immediately if a 401
// has been returned. All calls after this are not retried and a failing API call will stop the stream currently.
func (c *Client) StreamAllProjects(ctx context.Context, pageSize int, projectsChan chan<- Project) error {
	defer close(projectsChan)

	if err := c.checkGitLabauth(ctx); err != nil {
		if errors.Is(err, ErrUnauthorised) {
			return fmt.Errorf("stopping stream: %w", err)
		}
		return fmt.Errorf("stoppping stream: got error while checking gitlab auth: %w", err)
	}

	queryParams := url.Values{}
	queryParams.Set("pagination", "keyset")
	queryParams.Set("order_by", "id")
	queryParams.Set("per_page", strconv.Itoa(pageSize))
	queryParams.Set("simple", "true")

	nextRequestURL := fmt.Sprintf("%s/%s/%s?%s", c.Host, gitLabAPIPath, "projects", queryParams.Encode())

	for nextRequestURL != "" {
		resp, err := c.callGitLabAPI(ctx, nextRequestURL)
		if err != nil {
			return fmt.Errorf("stopping stream failed request: %w", err)
		}

		bodyBytes, err := readHTTPBody(resp.Body)
		if err != nil {
			return fmt.Errorf("stopping stream got bad response %s: failed to read response body: %s", resp.Status, err)
		}

		if resp.StatusCode > 299 {
			return fmt.Errorf("stopping stream, got bad response %s: %s", resp.Status, string(bodyBytes))
		}

		projects := make([]Project, 0)
		if err := json.Unmarshal(bodyBytes, &projects); err != nil {
			return fmt.Errorf("stopping stream, failed to unmarshal response: %w", err)
		}

		if len(projects) == 0 && string(bodyBytes) != "[]" {
			log.Printf("stopping stream: %s has no projects", c.Host)
			return nil
		}

		for _, p := range projects {
			projectsChan <- p
		}

		lhs := resp.Header.Get("Link")
		if lhs == "" {
			return nil
		}

		linkHeaders, err := parseLinkHeaders(lhs)
		if err != nil {
			return fmt.Errorf("failed to parse link header: %w", err)
		}

		nextLinkHeader := getNextLinkFromLinkHeaders(linkHeaders)
		if nextLinkHeader.link == "" {
			return errors.New("nextLinkHeader is empty")
		}

		nextRequestURL = nextLinkHeader.link
	}

	return nil
}

var ErrRawFileNotFound = errors.New("raw file was not found")

// GetRawFileFromProject wraps around the raw file endpoint of GitLab helping to fetch files from specific repos
// it will throw a typed ErrRawFileNotFound when it encounters a 404 response which you can errors.Is for to
// have cleaner logs.
func (c *Client) GetRawFileFromProject(ctx context.Context, projectID int, fileName, ref string) ([]byte, error) {
	queryParams := url.Values{}
	queryParams.Add("ref", ref)
	requestURL := fmt.Sprintf("%s/%s/projects/%d/repository/files/%s/raw?%s", c.Host, gitLabAPIPath, projectID, fileName, queryParams.Encode())

	resp, err := c.callGitLabAPI(ctx, requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw file %s on ref %s: %s", fileName, ref, err)
	}

	bodyBytes, err := readHTTPBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrRawFileNotFound
		}
		return nil, fmt.Errorf("failed to get raw file: %s", string(bodyBytes))
	}

	return bodyBytes, nil
}

var ErrUnauthorised = errors.New("client is unauthorised")

func (c *Client) checkGitLabauth(ctx context.Context) error {
	requestUrl := c.Host + "/" + gitLabAPIPath + "/version"

	call := func() error {
		resp, err := c.callGitLabAPI(ctx, requestUrl)
		if err != nil {
			return fmt.Errorf("failed to call %s: %w", requestUrl, err)
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return backoff.Permanent(ErrUnauthorised)
		}

		if resp.StatusCode > 299 {
			return fmt.Errorf("http error while calling %s: %s", requestUrl, resp.Status)
		}
		return nil
	}

	eb := &backoff.ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0.5,
		Multiplier:          1.5,
		MaxInterval:         5 * time.Second,
		MaxElapsedTime:      30 * time.Second,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
	if err := backoff.Retry(call, eb); err != nil {
		if errors.Is(err, ErrUnauthorised) {
			return err
		}

		return fmt.Errorf("exhausted all retries: %w", err)
	}

	return nil
}

// callGitLabAPI is the bare minimum implementation of the GitLab API for this
// crawler - it does not allow for anything other than GET requests
// it accepts an URL to enable proper keyset pagination which gives us complete URLs
func (c *Client) callGitLabAPI(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request to GitLab API: %w", err)
	}

	req.Header.Set(gitLabPrivateTokenHeader, c.Token)

	resp, err := c.HTTPDoer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitLab API on %s: %w", url, err)
	}

	return resp, nil
}

func readHTTPBody(bodyReader io.ReadCloser) ([]byte, error) {
	defer bodyReader.Close()

	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}

	return bodyBytes, nil
}

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

// parseLinkHeader tries to parse a list of RFC8288 compliant headers
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

var ErrNoRFC5988LinkHeader = errors.New("given string is not valid under RFC5988")

// parseLinkHeader is an incomplete parser for RFC8288 compliant header fields.
// It makes heavy assumptions around how GitLab uses web linking for keyset pagination.
// See https://docs.gitlab.com/ee/api/projects.html#list-all-projects for more info.
func parseLinkHeader(header string) (linkHeader, error) {
	var lh linkHeader

	elems := strings.Split(header, ";")

	if !strings.HasPrefix(elems[0], "<") {
		return linkHeader{}, ErrNoRFC5988LinkHeader
	}

	lh.link = elems[0][1 : len(elems[0])-1]

	_, err := url.Parse(lh.link)
	if err != nil {
		return linkHeader{}, fmt.Errorf("parsed link is not a valid URL: %w", err)
	}

	if !strings.HasPrefix(strings.TrimSpace(elems[1]), "rel") {
		return linkHeader{}, ErrNoRFC5988LinkHeader
	}

	trimmedRel := strings.TrimPrefix(strings.TrimSpace(elems[1]), "rel=")

	lh.rel = trimmedRel[1 : len(trimmedRel)-1]
	return lh, nil
}
